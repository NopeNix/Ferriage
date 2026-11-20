// Package console serves the interactive exec console over a
// WebSocket (Phase 3 item 2, ADR 0005). One exec session per
// connection: the peer creates a TTY exec, then pumps bytes
// between the socket and the hijacked endpoint in both
// directions. Control messages are JSON text frames:
//
//	{"type":"resize","width":N,"height":N}  — resize the TTY
//	{"type":"close"}                        — end the session
//
// Binary frames carry terminal bytes in both directions.
package console

import (
	"bufio"
	"context"
	"crypto/sha1"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"sync"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

// Client talks to the Docker daemon for exec.
type Client interface {
	ContainerExecCreate(ctx context.Context, containerID string, options container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecStart(ctx context.Context, execID string, config container.ExecStartOptions) error
	ContainerExecResize(ctx context.Context, execID string, options container.ResizeOptions) error
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

// control is a JSON text frame from the browser.
type control struct {
	Type   string `json:"type"`
	Width  int    `json:"width"`
	Height int    `json:"height"`
}

// Handler serves WS /stacks/{name}/services/{svc}/console.
type Handler struct {
	Lister    func(context.Context) ([]adopt.Stack, error)
	NewClient func() (Client, error)
	Done      chan struct{}
}

// Register wires the console route onto the peer mux.
func Register(mux *http.ServeMux) {
	h := &Handler{
		Done: make(chan struct{}),
		Lister: func(ctx context.Context) ([]adopt.Stack, error) {
			c, err := adopt.NewClient()
			if err != nil {
				return nil, err
			}
			defer func() { _ = c.Close() }()
			return adopt.List(ctx, c)
		},
		NewClient: func() (Client, error) { return adopt.NewClient() },
	}
	mux.HandleFunc("GET /stacks/{name}/services/{svc}/console", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	name, svc := r.PathValue("name"), r.PathValue("svc")
	if !validName(name) || !validName(svc) {
		hijackErr(w, http.StatusBadRequest, "bad stack or service name")
		return
	}
	key := r.Header.Get("Sec-WebSocket-Key")
	if key == "" {
		hijackErr(w, http.StatusBadRequest, "missing Sec-WebSocket-Key")
		return
	}
	hj, ok := w.(http.Hijacker)
	if !ok {
		hijackErr(w, http.StatusInternalServerError, "hijack unavailable")
		return
	}
	conn, brw, err := hj.Hijack()
	if err != nil {
		return
	}
	resp := "HTTP/1.1 101 Switching Protocols\r\n" +
		"Upgrade: websocket\r\n" +
		"Connection: Upgrade\r\n" +
		"Sec-WebSocket-Accept: " + wsAccept(key) + "\r\n\r\n"
	if _, err := brw.WriteString(resp); err != nil {
		_ = conn.Close()
		return
	}
	if err := brw.Flush(); err != nil {
		_ = conn.Close()
		return
	}

	id, err := resolve(r.Context(), h.Lister, name, svc)
	if err != nil {
		_ = writeFrame(brw, 0x1, []byte(errorFrame(err.Error())))
		_ = conn.Close()
		h.done()
		return
	}
	c, err := h.NewClient()
	if err != nil {
		_ = writeFrame(brw, 0x1, []byte(errorFrame("docker unavailable")))
		_ = conn.Close()
		h.done()
		return
	}
	defer func() {
		if closer, ok := any(c).(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()

	respExec, err := c.ContainerExecCreate(r.Context(), id, container.ExecOptions{
		Cmd: []string{"/bin/sh"}, Tty: true,
		AttachStdout: true, AttachStderr: true, AttachStdin: true,
	})
	if err != nil {
		msg := "exec create failed"
		if errdefs.IsNotFound(err) {
			msg = "container not found"
		}
		_ = writeFrame(brw, 0x1, []byte(errorFrame(msg)))
		_ = conn.Close()
		h.done()
		return
	}
	hijacked, err := c.ContainerExecAttach(r.Context(), respExec.ID, container.ExecAttachOptions{Tty: true})
	if err != nil {
		_ = writeFrame(brw, 0x1, []byte(errorFrame("exec attach failed")))
		_ = conn.Close()
		h.done()
		return
	}
	defer hijacked.Close()
	if err := c.ContainerExecStart(r.Context(), respExec.ID, container.ExecStartOptions{Tty: true}); err != nil {
		_ = writeFrame(brw, 0x1, []byte(errorFrame("exec start failed")))
		_ = conn.Close()
		h.done()
		return
	}

	sess := &session{c: c, execID: respExec.ID, conn: conn, brw: brw, child: hijacked.Conn}
	sess.run()
	h.done()
}

func (h *Handler) done() {
	select {
	case <-h.Done:
	default:
		close(h.Done)
	}
}

func errorFrame(msg string) string {
	q, _ := json.Marshal(map[string]string{"type": "error", "message": msg})
	return string(q)
}

// session pumps one WebSocket connection <-> one exec attach.
type session struct {
	c      Client
	execID string
	conn   net.Conn
	brw    *bufio.ReadWriter
	child  net.Conn
	mu     sync.Mutex
	once   sync.Once
}

func (s *session) close() {
	s.once.Do(func() { _ = s.conn.Close() })
}

func (s *session) send(payload []byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	_ = writeFrame(s.brw, 0x2, payload)
}

func (s *session) run() {
	// exec output -> socket
	go func() {
		buf := make([]byte, 16<<10)
		for {
			n, err := s.child.Read(buf)
			if n > 0 {
				s.send(append([]byte(nil), buf[:n]...))
			}
			if err != nil {
				s.close()
				return
			}
		}
	}()
	// socket -> exec input / control
	for {
		payload, isText, err := readFrame(s.brw)
		if err != nil {
			s.close()
			return
		}
		if isText {
			var ctrl control
			if json.Unmarshal(payload, &ctrl) == nil && ctrl.Type != "" {
				switch ctrl.Type {
				case "resize":
					_ = s.c.ContainerExecResize(context.Background(), s.execID, container.ResizeOptions{Width: uint(ctrl.Width), Height: uint(ctrl.Height)})
				case "close":
					s.close()
					return
				}
				continue
			}
		}
		_, _ = s.child.Write(payload)
	}
}

func validName(s string) bool {
	if s == "" || s == "." || s == ".." {
		return false
	}
	for _, r := range s {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_', r == '.':
		default:
			return false
		}
	}
	return true
}

func resolve(ctx context.Context, list func(context.Context) ([]adopt.Stack, error), stackName, svc string) (string, error) {
	stacks, err := list(ctx)
	if err != nil {
		return "", err
	}
	for _, s := range stacks {
		if s.Name != stackName {
			continue
		}
		for _, sv := range s.Services {
			if sv.Name == svc {
				return sv.Container, nil
			}
		}
	}
	return "", fmt.Errorf("service %q not found in stack %q", svc, stackName)
}

func hijackErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_, _ = w.Write([]byte(`{"error":"` + msg + `"}`))
}

// wsAccept computes Sec-WebSocket-Accept for a handshake key.
func wsAccept(key string) string {
	h := sha1Sum([]byte(key + wsGUID))
	return base64Encode(h)
}

const wsGUID = "258EAFA5-E914-47DA-95CA-C5AB0DC85B11"

func sha1Sum(b []byte) []byte {
	h := sha1.New()
	_, _ = h.Write(b)
	return h.Sum(nil)
}

func base64Encode(b []byte) string { return base64.StdEncoding.EncodeToString(b) }
