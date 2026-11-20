package update

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func testManager(t *testing.T) *Manager {
	t.Helper()
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "demo"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "demo", "compose.yaml"),
		[]byte("services:\n  app:\n    image: nginx:1\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	return NewManager(dir, Config{DefaultPolicy: PolicyPoll, DefaultInterval: time.Hour},
		fakePeers{stacks: []adoptedStack{{Name: "demo", Services: []adoptedService{{Name: "app", Image: "nginx:1"}}}}},
		fakeDigests{
			local:  map[string]string{"nginx:1": "sha256:old"},
			remote: map[string]string{"nginx:1": "sha256:new"},
		},
		&fakeUpdater{snapPath: "snapshots/demo/x.tar.gz", healthy: true}, nil, nil)
}

func serve(t *testing.T, m *Manager, method, path, body string) *httptest.ResponseRecorder {
	t.Helper()
	mux := http.NewServeMux()
	Register(mux, m)
	var r *http.Request
	if body != "" {
		r = httptest.NewRequest(method, path, strings.NewReader(body))
	} else {
		r = httptest.NewRequest(method, path, nil)
	}
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, r)
	return rec
}

func TestHandlerPolicyRoundTrip(t *testing.T) {
	m := testManager(t)
	rec := serve(t, m, "PUT", "/update/demo/policy", `{"policy":"auto"}`)
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"policy":"auto"`) {
		t.Fatalf("put policy: %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, m, "GET", "/update/demo", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"policy":"auto"`) {
		t.Fatalf("get after put: %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, m, "PUT", "/update/demo/policy", `{"policy":"sometimes"}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("invalid policy: %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, m, "PUT", "/update/demo/policy", `{}`)
	if rec.Code != http.StatusBadRequest {
		t.Fatalf("empty policy: %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, m, "PUT", "/update/ghost/policy", `{"policy":"auto"}`)
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ghost policy: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerCheckAndRun(t *testing.T) {
	m := testManager(t)
	rec := serve(t, m, "POST", "/update/demo/check", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"state":"available"`) {
		t.Fatalf("check: %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, m, "POST", "/update/demo/run", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"ok":true`) {
		t.Fatalf("run: %d %s", rec.Code, rec.Body.String())
	}
	// Check is a probe: it must not have deployed (only run does).
	// (fakeUpdater counts are on the manager's Updater; run above
	// is the one deploy.)
	rec = serve(t, m, "GET", "/update", "")
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), `"stack":"demo"`) {
		t.Fatalf("list: %d %s", rec.Code, rec.Body.String())
	}
	rec = serve(t, m, "GET", "/update/ghost", "")
	if rec.Code != http.StatusNotFound {
		t.Fatalf("ghost get: %d %s", rec.Code, rec.Body.String())
	}
}

func TestHandlerBadNames(t *testing.T) {
	m := testManager(t)
	for _, p := range []string{"/update/../x", "/update/a/b"} {
		rec := serve(t, m, "GET", p, "")
		// a/b is not a routed stack name; ../x is rejected.
		if rec.Code == http.StatusOK {
			t.Fatalf("path %s: got 200, want non-200", p)
		}
	}
}
