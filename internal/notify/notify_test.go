package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// recordingShoutrrr stands up a local HTTP server shaped like a
// Shoutrrr generic webhook: it captures the last request body so
// tests can prove the message that would leave the peer.
type recordingShoutrrr struct {
	mu       chan string
	lastBody string
}

func newRecorder() *recordingShoutrrr {
	return &recordingShoutrrr{mu: make(chan string, 8)}
}

func (r *recordingShoutrrr) handler() http.HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) {
		b := make([]byte, 4096)
		n, _ := req.Body.Read(b)
		r.lastBody = string(b[:n])
		r.mu <- r.lastBody
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	}
}

func TestParseURLs(t *testing.T) {
	got := ParseURLs(" https://a , ,https://b, ")
	if len(got) != 2 || got[0] != "https://a" || got[1] != "https://b" {
		t.Errorf("ParseURLs = %q", got)
	}
	if got := ParseURLs(""); len(got) != 0 {
		t.Errorf("ParseURLs empty = %q", got)
	}
}

func TestDisabledIsNoop(t *testing.T) {
	n, err := New(nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if n.Enabled() {
		t.Error("nil URLs should be disabled")
	}
	// Must not panic or block.
	n.Notify(context.Background(), EventDeployOK, "demo", "healthy")
	if err := n.Test(context.Background()); err != ErrDisabled {
		t.Errorf("Test on disabled = %v, want ErrDisabled", err)
	}
	var nilN *Notifier
	nilN.Notify(context.Background(), EventDeployOK, "demo", "healthy") // nil receiver no-op
	if nilN.Enabled() || nilN.Channels() != 0 {
		t.Error("nil Notifier should report disabled/0")
	}
}

func TestNotifySendsMessage(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()

	// Shoutrrr "generic" scheme posts JSON to the URL; point it
	// at the local recorder with TLS disabled (plain HTTP test).
	url := "generic://" + strings.TrimPrefix(srv.URL, "http://") + "?disabletls=yes"
	n, err := New([]string{url}, func() string { return "0123456789abcdef" })
	if err != nil {
		t.Fatal(err)
	}
	if !n.Enabled() || n.Channels() != 1 {
		t.Fatalf("enabled=%v channels=%d", n.Enabled(), n.Channels())
	}
	n.Notify(context.Background(), EventDeployOK, "demo", "healthy")
	select {
	case body := <-rec.mu:
		if !strings.Contains(body, "deploy.ok") || !strings.Contains(body, "demo") {
			t.Errorf("body = %s, want deploy.ok + demo", body)
		}
		if !strings.Contains(body, "0123456789ab") {
			t.Errorf("body = %s, want peer prefix", body)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("no notification within 5s")
	}
	// Status reflects the send, without leaking the URL. The
	// delivery is async; poll until it lands.
	var st Status
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		st = n.Status()
		if st.LastEvent == string(EventDeployOK) {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if !st.Enabled || st.Channels != 1 || st.LastEvent != string(EventDeployOK) || st.LastErr != "" {
		t.Errorf("status = %+v", st)
	}
	raw, _ := json.Marshal(st)
	if strings.Contains(string(raw), "generic://") {
		t.Errorf("status leaks URL: %s", raw)
	}
}

func TestTestEndpoint(t *testing.T) {
	rec := newRecorder()
	srv := httptest.NewServer(rec.handler())
	defer srv.Close()
	url := "generic://" + strings.TrimPrefix(srv.URL, "http://") + "?disabletls=yes"
	n, _ := New([]string{url}, nil)

	mux := http.NewServeMux()
	Register(mux, n)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Get(ts.URL + "/notify")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /notify = %d", resp.StatusCode)
	}
	var st map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&st); err != nil {
		t.Fatal(err)
	}
	if st["enabled"] != true || st["channels"] != float64(1) {
		t.Errorf("status = %v", st)
	}

	resp2, err := http.Post(ts.URL+"/notify/test", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp2.Body.Close() }()
	if resp2.StatusCode != http.StatusOK {
		t.Fatalf("POST /notify/test = %d", resp2.StatusCode)
	}
	select {
	case <-rec.mu:
	case <-time.After(5 * time.Second):
		t.Fatal("test send did not reach channel")
	}
}

func TestTestEndpointDisabled(t *testing.T) {
	n, _ := New(nil, nil)
	mux := http.NewServeMux()
	Register(mux, n)
	ts := httptest.NewServer(mux)
	defer ts.Close()

	resp, err := http.Post(ts.URL+"/notify/test", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("disabled test = %d, want 503", resp.StatusCode)
	}
}

func TestNotifyNeverBlocksOnDeadChannel(t *testing.T) {
	// Unroutable address: Notify must return immediately and the
	// failure must surface in status, not as a deploy error.
	n, _ := New([]string{"generic://127.0.0.1:1/hook"}, nil)
	start := time.Now()
	n.Notify(context.Background(), EventDeployFailed, "demo", "boom")
	if time.Since(start) > time.Second {
		t.Fatalf("Notify blocked %s", time.Since(start))
	}
	deadline := time.Now().Add(15 * time.Second)
	for time.Now().Before(deadline) {
		if st := n.Status(); st.LastEvent == string(EventDeployFailed) {
			if st.LastErr == "" {
				t.Errorf("dead channel should record an error, status = %+v", st)
			}
			return
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatal("dead-channel send never recorded a status")
}
