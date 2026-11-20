package exec

import (
	"bufio"
	"context"
	"encoding/binary"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
)

func muxFrame(stream byte, msg string) []byte {
	b := make([]byte, 8+len(msg))
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:8], uint32(len(msg)))
	copy(b[8:], msg)
	return b
}

type fakeExecClient struct {
	out     []byte
	exit    int
	running bool
}

func (f *fakeExecClient) ContainerExecCreate(_ context.Context, _ string, _ container.ExecOptions) (container.ExecCreateResponse, error) {
	return container.ExecCreateResponse{ID: "exec-1"}, nil
}

func (f *fakeExecClient) ContainerExecAttach(_ context.Context, _ string, _ container.ExecAttachOptions) (types.HijackedResponse, error) {
	return types.HijackedResponse{
		Conn:   &fakeConn{reader: bufio.NewReader(strings.NewReader(string(f.out)))},
		Reader: bufio.NewReader(strings.NewReader(string(f.out))),
	}, nil
}

func (f *fakeExecClient) ContainerExecStart(context.Context, string, container.ExecStartOptions) error {
	return nil
}

func (f *fakeExecClient) ContainerExecInspect(context.Context, string) (container.ExecInspect, error) {
	return container.ExecInspect{ExecID: "exec-1", Running: f.running, ExitCode: f.exit}, nil
}

func (f *fakeExecClient) ContainerInspect(context.Context, string) (container.InspectResponse, error) {
	return container.InspectResponse{}, nil
}

type fakeConn struct {
	reader *bufio.Reader
}

func (c *fakeConn) Read(p []byte) (int, error)  { return c.reader.Read(p) }
func (c *fakeConn) Write(p []byte) (int, error) { return len(p), nil }
func (c *fakeConn) Close() error                { return nil }
func (c *fakeConn) LocalAddr() net.Addr         { return nil }
func (c *fakeConn) RemoteAddr() net.Addr        { return nil }
func (c *fakeConn) SetDeadline(time.Time) error { return nil }
func (c *fakeConn) SetReadDeadline(time.Time) error {
	return nil
}
func (c *fakeConn) SetWriteDeadline(time.Time) error { return nil }

func TestRunDemuxesOutput(t *testing.T) {
	raw := append(muxFrame(1, "out-one"), muxFrame(2, "err-one")...)
	res, err := Run(context.Background(), &fakeExecClient{out: raw, exit: 3}, "cid", []string{"echo", "x"}, false)
	if err != nil {
		t.Fatal(err)
	}
	if res.ExitCode != 3 || res.ExecID != "exec-1" || !strings.Contains(res.Output, "out-one") || !strings.Contains(res.Output, "err-one") {
		t.Fatalf("result: %+v", res)
	}
}

func TestRunEmptyCmd(t *testing.T) {
	if _, err := Run(context.Background(), &fakeExecClient{}, "cid", nil, false); err == nil {
		t.Fatal("expected error")
	}
}

func TestHandlerBadRequest(t *testing.T) {
	h := &Handler{
		Lister: func(context.Context) ([]adopt.Stack, error) {
			return []adopt.Stack{{Name: "s", Services: []adopt.Service{{Name: "app", Container: "cid"}}}}, nil
		},
		NewClient: func() (Client, error) { return &fakeExecClient{}, nil },
	}
	mux := http.NewServeMux()
	mux.HandleFunc("POST /stacks/{name}/services/{svc}/exec", h.serve)

	rec := newRecorder()
	mux.ServeHTTP(rec, newRequest("POST", "/stacks/s/services/app/exec", `{"cmd":[]}`))
	if rec.code != http.StatusBadRequest {
		t.Fatalf("empty cmd: %d", rec.code)
	}
	rec = newRecorder()
	mux.ServeHTTP(rec, newRequest("POST", "/stacks/s/services/ghost/exec", `{"cmd":["ls"]}`))
	if rec.code != http.StatusNotFound {
		t.Fatalf("unknown svc: %d", rec.code)
	}
}

// tiny recorder/request shims keep this test independent of
// httptest's Result; enough for status checks.
type rec struct {
	code int
	body string
}

func newRecorder() *rec { return &rec{} }

func (r *rec) Header() http.Header         { return http.Header{} }
func (r *rec) Write(p []byte) (int, error) { r.body += string(p); return len(p), nil }
func (r *rec) WriteHeader(code int)        { r.code = code }
func newRequest(method, path, body string) *http.Request {
	req, _ := http.NewRequest(method, path, strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	return req
}

var _ = io.Discard
