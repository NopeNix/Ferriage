// Package logs serves bounded, non-follow container log tails for a
// stack's services (Phase 3 item 2: logs/stats/exec). Logs are a
// transport artifact, not state, so state-sync has nothing to do
// with them (ADR 0005): the peer reads the daemon directly and
// never replicates log bytes.
package logs

import (
	"bufio"
	"context"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/NopeNix/Ferriage/internal/api"
	"github.com/containerd/errdefs"
	"github.com/docker/docker/api/types/container"
)

// Client talks to the Docker daemon for logs and container lookup.
type Client interface {
	ContainerLogs(ctx context.Context, containerID string, options container.LogsOptions) (io.ReadCloser, error)
	ContainerInspect(ctx context.Context, containerID string) (container.InspectResponse, error)
}

// Opts tunes a log tail request.
type Opts struct {
	Tail  int
	Since string
}

// Line is one parsed log frame: stream name, timestamp, message.
type Line struct {
	Ts      string
	Stream  string
	Message string
}

// parseFrames splits a raw Docker log stream (multiplexed frames
// when not TTY) into lines, preserving order and stream. A frame
// larger than 1MB is an error rather than a silent truncation.
func parseFrames(r io.Reader) ([]Line, error) {
	br := bufio.NewReader(r)
	var out []Line
	for {
		hdr := make([]byte, 8)
		if _, err := io.ReadFull(br, hdr); err != nil {
			if err == io.EOF {
				return out, nil
			}
			return out, fmt.Errorf("logs: read frame header: %w", err)
		}
		size := binary.BigEndian.Uint32(hdr[4:8])
		if size > 1<<20 {
			return out, fmt.Errorf("logs: frame too large (%d bytes)", size)
		}
		payload := make([]byte, size)
		if _, err := io.ReadFull(br, payload); err != nil {
			return out, fmt.Errorf("logs: read frame: %w", err)
		}
		stream := "stdout"
		if hdr[0] == 2 {
			stream = "stderr"
		}
		raw := string(payload)
		ts := ""
		msg := raw
		if i := strings.IndexByte(raw, ' '); i > 0 {
			if isTS(raw[:i]) {
				ts, msg = raw[:i], raw[i+1:]
			}
		}
		for _, part := range strings.Split(strings.TrimSuffix(msg, "\n"), "\n") {
			out = append(out, Line{Ts: ts, Stream: stream, Message: part})
		}
	}
}

// isTS reports whether s looks like an RFC3339 (Docker) timestamp.
func isTS(s string) bool {
	return len(s) >= 20 && s[4] == '-' && s[7] == '-'
}

// parseTTY splits raw TTY output (no frame headers) into lines.
func parseTTY(r io.Reader) []Line {
	sc := bufio.NewScanner(r)
	sc.Buffer(make([]byte, 0, 64<<10), 1<<20)
	var out []Line
	for sc.Scan() {
		out = append(out, Line{Stream: "stdout", Message: sc.Text()})
	}
	return out
}

// isTTY reports whether the container was created with a TTY.
func isTTY(ctx context.Context, c Client, id string) bool {
	insp, err := c.ContainerInspect(ctx, id)
	if err != nil || insp.ContainerJSONBase == nil || insp.Config == nil {
		return false
	}
	return insp.Config.Tty
}

// Tail returns up to opts.Tail lines from the container's logs
// (newest last). It never follows.
func Tail(ctx context.Context, c Client, id string, opts Opts) ([]Line, bool, error) {
	if opts.Tail <= 0 {
		opts.Tail = 200
	}
	rc, err := c.ContainerLogs(ctx, id, container.LogsOptions{
		ShowStdout: true,
		ShowStderr: true,
		Timestamps: true,
		Tail:       strconv.Itoa(opts.Tail),
		Since:      opts.Since,
	})
	if err != nil {
		return nil, false, err
	}
	defer func() { _ = rc.Close() }()
	var got []Line
	if isTTY(ctx, c, id) {
		got = parseTTY(rc)
	} else {
		got, err = parseFrames(rc)
		if err != nil {
			return nil, false, err
		}
	}
	if len(got) > opts.Tail {
		return got[len(got)-opts.Tail:], true, nil
	}
	return got, false, nil
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

// Handler serves GET /stacks/{name}/services/{svc}/logs.
type Handler struct {
	Lister    func(context.Context) ([]adopt.Stack, error)
	NewClient func() (Client, error)
}

// Register wires the logs route onto the peer mux.
func Register(mux *http.ServeMux) {
	h := &Handler{
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
	mux.HandleFunc("GET /stacks/{name}/services/{svc}/logs", h.serve)
}

func (h *Handler) serve(w http.ResponseWriter, r *http.Request) {
	name, svc := r.PathValue("name"), r.PathValue("svc")
	if !validName(name) || !validName(svc) {
		writeErr(w, http.StatusBadRequest, "bad stack or service name")
		return
	}
	tail := 200
	if v := r.URL.Query().Get("tail"); v != "" {
		n, err := strconv.Atoi(v)
		if err != nil || n < 1 || n > 10000 {
			writeErr(w, http.StatusBadRequest, "tail must be 1..10000")
			return
		}
		tail = n
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
	lines, truncated, err := Tail(r.Context(), c, id, Opts{Tail: tail, Since: r.URL.Query().Get("since")})
	if err != nil {
		if errdefs.IsNotFound(err) {
			writeErr(w, http.StatusNotFound, "container not found")
			return
		}
		writeErr(w, http.StatusBadGateway, err.Error())
		return
	}
	out := make([]api.LogLine, 0, len(lines))
	for _, l := range lines {
		out = append(out, api.LogLine{Ts: l.Ts, Stream: l.Stream, Message: l.Message})
	}
	w.Header().Set("Content-Type", "application/json")
	_ = json.NewEncoder(w).Encode(api.LogsResponse{Container: id, Lines: out, Truncated: truncated})
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
