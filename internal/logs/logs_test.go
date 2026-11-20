package logs

import (
	"context"
	"encoding/binary"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/NopeNix/Ferriage/internal/adopt"
	"github.com/docker/docker/api/types/container"
)

// frame builds one multiplexed log frame (stream, timestamp, message).
func frame(stream byte, ts, msg string) []byte {
	payload := msg
	if ts != "" {
		payload = ts + " " + msg + "\n"
	}
	b := make([]byte, 8+len(payload))
	b[0] = stream
	binary.BigEndian.PutUint32(b[4:8], uint32(len(payload)))
	copy(b[8:], payload)
	return b
}

type fakeLogsClient struct {
	raw     []byte
	tty     bool
	logsErr error
}

func (f *fakeLogsClient) ContainerLogs(_ context.Context, _ string, _ container.LogsOptions) (io.ReadCloser, error) {
	if f.logsErr != nil {
		return nil, f.logsErr
	}
	return io.NopCloser(strings.NewReader(string(f.raw))), nil
}

func (f *fakeLogsClient) ContainerInspect(_ context.Context, _ string) (container.InspectResponse, error) {
	resp := container.InspectResponse{}
	resp.ContainerJSONBase = &container.ContainerJSONBase{}
	resp.Config = &container.Config{Tty: f.tty}
	return resp, nil
}

func TestParseFramesMultiplexed(t *testing.T) {
	raw := append(frame(1, "2026-11-20T00:00:00.000000000Z", "hello"), frame(2, "2026-11-20T00:00:01.000000000Z", "oops")...)
	got, err := parseFrames(strings.NewReader(string(raw)))
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 || got[0].Message != "hello" || got[0].Stream != "stdout" || got[1].Stream != "stderr" {
		t.Fatalf("unexpected lines: %+v", got)
	}
	if got[0].Ts == "" {
		t.Fatal("expected timestamp")
	}
}

func TestParseFramesTruncatedHeader(t *testing.T) {
	_, err := parseFrames(strings.NewReader("\x01\x00"))
	if err == nil {
		t.Fatal("expected error on truncated header")
	}
}

func TestParseFramesOversized(t *testing.T) {
	hdr := make([]byte, 8)
	binary.BigEndian.PutUint32(hdr[4:8], 1<<21)
	_, err := parseFrames(strings.NewReader(string(hdr)))
	if err == nil {
		t.Fatal("expected error on oversized frame")
	}
}

func TestTailCapsAtTail(t *testing.T) {
	raw := frame(1, "", "one")
	raw = append(raw, frame(1, "", "two")...)
	raw = append(raw, frame(1, "", "three")...)
	lines, truncated, err := Tail(context.Background(), &fakeLogsClient{raw: raw}, "abc", Opts{Tail: 2})
	if err != nil {
		t.Fatal(err)
	}
	if !truncated || len(lines) != 2 || lines[0].Message != "two" {
		t.Fatalf("expected last two + truncated, got %+v truncated=%v", lines, truncated)
	}
}

func TestTailTTY(t *testing.T) {
	lines, truncated, err := Tail(context.Background(), &fakeLogsClient{raw: []byte("a\nb\nc\n"), tty: true}, "abc", Opts{Tail: 10})
	if err != nil {
		t.Fatal(err)
	}
	if truncated || len(lines) != 3 {
		t.Fatalf("tty tail: %+v truncated=%v", lines, truncated)
	}
}

func TestHandlerBadTailAndNotFound(t *testing.T) {
	h := &Handler{
		Lister: func(context.Context) ([]adopt.Stack, error) {
			return []adopt.Stack{{Name: "s", Services: []adopt.Service{{Name: "app", Container: "cid"}}}}, nil
		},
		NewClient: func() (Client, error) {
			return &fakeLogsClient{raw: frame(1, "", "hi")}, nil
		},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stacks/{name}/services/{svc}/logs", h.serve)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/s/services/app/logs?tail=0", nil))
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("tail=0: status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/s/services/ghost/logs", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("unknown svc: status %d", rec.Code)
	}

	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/s/services/app/logs", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"message":"hi"`) {
		t.Fatalf("ok case: status %d body %s", rec.Code, rec.Body.String())
	}
}
