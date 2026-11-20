package deploy

import (
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestDeployMissingStack(t *testing.T) {
	mux := http.NewServeMux()
	Register(mux, t.TempDir())
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(srv.URL+"/stacks/nothere/deploy", "application/octet-stream", nil)
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("deploy missing stack: got %d, want 404", resp.StatusCode)
	}
}

func TestDeployLive(t *testing.T) {
	if os.Getenv("FERRIAGE_LIVE") == "" {
		t.Skip("set FERRIAGE_LIVE=1 against a real daemon + docker CLI")
	}
	stateDir := t.TempDir()
	dir := filepath.Join(stateDir, "demo")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	compose := []byte("services:\n  app:\n    image: alpine:3.20\n    command: [\"sleep\", \"1000\"]\n")
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), compose, 0o644); err != nil {
		t.Fatal(err)
	}

	h := &Handler{StateDir: stateDir, Cfg: Config{Compose: "docker", Timeout: 30 * time.Second, Poll: 500 * time.Millisecond}}
	res, err := Deploy(t.Context(), dir, h.Cfg)
	if err != nil {
		t.Fatalf("Deploy: %v", err)
	}
	if !res.Healthy {
		t.Fatalf("not healthy: %+v", res)
	}
	t.Logf("deployed healthy: %+v", res.Services)
}
