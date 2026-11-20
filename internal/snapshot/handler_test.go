package snapshot

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandlerExportImport(t *testing.T) {
	stateDir := t.TempDir()
	writeTestStack(t, stateDir, "demo")
	volRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(volRoot, "d.txt"), []byte("payload"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{volumes: map[string]string{"demo_data": volRoot}, labels: map[string]string{"demo_data": "demo"}}

	// Export via HTTP (needs a real daemon client — replaced by direct
	// Export call below if no daemon). Instead exercise the handler's
	// shape with a missing-stack 404 and a method check.
	mux := http.NewServeMux()
	Register(mux, stateDir, "peer-test")
	srv := httptest.NewServer(mux)
	defer srv.Close()

	// Missing stack -> 404.
	resp, err := http.Post(srv.URL+"/stacks/nothere/export", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("export missing stack: got %d, want 404", resp.StatusCode)
	}

	// Wrong method -> 405.
	resp, err = http.Get(srv.URL + "/stacks/demo/export")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("export GET: got %d, want 405", resp.StatusCode)
	}

	// Round trip through Export/Restore with the fake backend.
	var buf bytes.Buffer
	if err := Export(context.Background(), &buf, stateDir, "demo", "peer-test", backend); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if err := backend.VolumeRemove(context.Background(), "demo_data", true); err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	m, err := Restore(context.Background(), bytes.NewReader(buf.Bytes()), fresh, "demo", backend)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if m.Stack != "demo" {
		t.Errorf("stack = %q", m.Stack)
	}
}
