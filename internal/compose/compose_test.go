package compose

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGetPutRoundTrip(t *testing.T) {
	dir := t.TempDir()
	stack := filepath.Join(dir, "s")
	if err := os.MkdirAll(stack, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stack, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	h := &Handler{StateDir: dir}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stacks/{name}/compose", h.serveGet)
	mux.HandleFunc("PUT /stacks/{name}/compose", h.servePut)

	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/s/compose", nil))
	if rec.Code != http.StatusOK || !strings.Contains(rec.Body.String(), "services: {}") {
		t.Fatalf("get: %d %s", rec.Code, rec.Body.String())
	}

	body := `{"content":"services:\n  app:\n    image: x\n"}`
	rec = httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("PUT", "/stacks/s/compose", strings.NewReader(body)))
	if rec.Code != http.StatusOK {
		t.Fatalf("put: %d %s", rec.Code, rec.Body.String())
	}
	data, err := os.ReadFile(filepath.Join(stack, "compose.yaml"))
	if err != nil || !strings.Contains(string(data), "image: x") {
		t.Fatalf("file: %q err=%v", data, err)
	}
}

func TestMissingComposeIs404(t *testing.T) {
	h := &Handler{StateDir: t.TempDir()}
	mux := http.NewServeMux()
	mux.HandleFunc("GET /stacks/{name}/compose", h.serveGet)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, httptest.NewRequest("GET", "/stacks/ghost/compose", nil))
	if rec.Code != http.StatusNotFound {
		t.Fatalf("status %d", rec.Code)
	}
}
