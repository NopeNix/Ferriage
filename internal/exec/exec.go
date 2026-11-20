// Package exec runs one-shot commands inside a running container
// (Phase 3 item 2). One-shot only: the interactive console lives
// in internal/console over WebSocket (ADR 0005).
package exec

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/pkg/stdcopy"
)

// Client talks to the Docker daemon for exec.
type Client interface {
	ContainerExecCreate(ctx context.Context, containerID string, options container.ExecOptions) (container.ExecCreateResponse, error)
	ContainerExecAttach(ctx context.Context, execID string, config container.ExecAttachOptions) (types.HijackedResponse, error)
	ContainerExecStart(ctx context.Context, execID string, config container.ExecStartOptions) error
	ContainerExecInspect(ctx context.Context, execID string) (container.ExecInspect, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

// Result is one completed exec call.
type Result struct {
	ExecID    string
	Running   bool
	ExitCode  int
	Output    string
	Truncated bool
}

const maxOutput = 1 << 20 // 1MB cap, same as logs frames

// Run creates, attaches, starts, and collects one exec call.
func Run(ctx context.Context, c Client, containerID string, cmd []string, tty bool) (Result, error) {
	if len(cmd) == 0 {
		return Result{}, fmt.Errorf("exec: empty cmd")
	}
	resp, err := c.ContainerExecCreate(ctx, containerID, container.ExecOptions{
		Cmd:          cmd,
		AttachStdout: true,
		AttachStderr: true,
		Tty:          tty,
	})
	if err != nil {
		return Result{}, err
	}
	execID := resp.ID

	var hijacked types.HijackedResponse
	hijacked, err = c.ContainerExecAttach(ctx, execID, container.ExecAttachOptions{Tty: tty})
	if err != nil {
		return Result{}, err
	}
	defer hijacked.Close()

	if err := c.ContainerExecStart(ctx, execID, container.ExecStartOptions{Tty: tty}); err != nil {
		return Result{}, err
	}

	var buf bytes.Buffer
	truncated := false
	limited := &limitedWriter{w: &buf, n: maxOutput, truncated: &truncated}
	if tty {
		_, _ = io.Copy(limited, hijacked.Reader)
	} else {
		_, _ = stdcopy.StdCopy(limited, limited, hijacked.Reader)
	}

	insp, err := c.ContainerExecInspect(ctx, execID)
	if err != nil {
		return Result{}, err
	}
	return Result{
		ExecID:    execID,
		Running:   insp.Running,
		ExitCode:  insp.ExitCode,
		Output:    buf.String(),
		Truncated: truncated,
	}, nil
}

// limitedWriter caps output at n bytes, marking truncated.
type limitedWriter struct {
	w         io.Writer
	n         int64
	truncated *bool
	written   int64
}

func (l *limitedWriter) Write(p []byte) (int, error) {
	if l.written >= l.n {
		*l.truncated = true
		return len(p), nil
	}
	remain := l.n - l.written
	if int64(len(p)) > remain {
		*l.truncated = true
		n, err := l.w.Write(p[:remain])
		l.written += int64(n)
		return n, err
	}
	l.written += int64(len(p))
	return l.w.Write(p)
}

// resolve picks a container ID for a stack+service via adoption.
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

// Handler serves POST /stacks/{name}/services/{svc}/exec.
type Handler struct {
	Lister    func(context.Context) ([]adopt.Stack, error)
	NewClient func() (Client, error)
	Timeout   time.Duration
}

// Register wires the exec route onto the peer mux.
func Register(mux *http.ServeMux) {
	h := &Handler{
		Timeout: 120 * time.Second,
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
	mux.HandleFunc("POST /stacks/{name}/services/{svc}/exec", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	name, svc := r.PathValue("name"), r.PathValue("svc")
	if !validName(name) || !validName(svc) {
		writeErr(w, http.StatusBadRequest, "bad stack or service name")
		return
	}
	var req api.ExecRequest
	if err := json.NewDecoder(http.MaxBytesReader(w, r.Body, 1<<20)).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad exec request")
		return
	}
	if len(req.Cmd) == 0 {
		writeErr(w, http.StatusBadRequest, "cmd must not be empty")
		return
	}
	id, err := resolve(r.Context(), h.Lister, name, svc)
	if err != nil {
		writeErr(w, http.StatusNotFound, err.Error())
		return
	}
	c, err := h.NewClient()
	if err != nil {
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	defer func() {
		if closer, ok := any(c).(interface{ Close() error }); ok {
			_ = closer.Close()
		}
	}()
	timeout := h.Timeout
	if timeout <= 0 {
		timeout = 120 * time.Second
	}
	ctx, cancel := context.WithTimeout(r.Context(), timeout)
	defer cancel()
	res, err := Run(ctx, c, id, req.Cmd, req.Tty)
	if err != nil {
		if errdefs.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "container not found")
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.ExecResponse{
		ExecID:    res.ExecID,
		Running:   res.Running,
		ExitCode:  res.ExitCode,
		Output:    res.Output,
		Truncated: res.Truncated,
	})
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

func writeErr(w http.ResponseWriter, status int, msg string) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(api.ErrorResponse{Error: msg})
}
