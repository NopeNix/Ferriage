package backup

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/volume"
)

// fakeBackend fakes the daemon's volume surface (same shape as
// internal/snapshot's test fake).
type fakeBackend struct {
	volumes map[string]string
}

func (f *fakeBackend) VolumeList(ctx context.Context, opts volume.ListOptions) (volume.ListResponse, error) {
	var out []*volume.Volume
	for name, mp := range f.volumes {
		out = append(out, &volume.Volume{Name: name, Mountpoint: mp, Labels: map[string]string{"com.docker.compose.project": "demo"}})
	}
	return volume.ListResponse{Volumes: out}, nil
}

func (f *fakeBackend) VolumeInspect(ctx context.Context, id string) (volume.Volume, error) {
	mp, ok := f.volumes[id]
	if !ok {
		return volume.Volume{}, fmt.Errorf("no such volume")
	}
	return volume.Volume{Name: id, Mountpoint: mp}, nil
}

func (f *fakeBackend) VolumeCreate(ctx context.Context, opts volume.CreateOptions) (volume.Volume, error) {
	if _, exists := f.volumes[opts.Name]; exists {
		return volume.Volume{}, fmt.Errorf("already exists")
	}
	mp, err := os.MkdirTemp("", "mp-*")
	if err != nil {
		return volume.Volume{}, err
	}
	f.volumes[opts.Name] = mp
	return volume.Volume{Name: opts.Name, Mountpoint: mp}, nil
}

func (f *fakeBackend) VolumeRemove(ctx context.Context, volumeID string, force bool) error {
	delete(f.volumes, volumeID)
	return nil
}

func (f *fakeBackend) VolumeCopyOut(ctx context.Context, volumeName string, w io.Writer) error {
	dir, ok := f.volumes[volumeName]
	if !ok {
		return fmt.Errorf("no such volume")
	}
	cmd := exec.CommandContext(ctx, "tar", "-cf", "-", "-C", dir, ".")
	cmd.Stdout = w
	return cmd.Run()
}

func (f *fakeBackend) VolumeCopyIn(ctx context.Context, volumeName string, r io.Reader) error {
	dir, ok := f.volumes[volumeName]
	if !ok {
		return fmt.Errorf("no such volume")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	cmd := exec.CommandContext(ctx, "tar", "-xf", "-", "-C", dir)
	cmd.Stdin = r
	return cmd.Run()
}

// TestHandler needs-no-restic: the status surface answers without
// a restic binary (repo uninitialized + password_set false).
func TestHandlerStatusWithoutRestic(t *testing.T) {
	stateDir := t.TempDir()
	mux := http.NewServeMux()
	Register(mux, stateDir, nil)
	req := httptest.NewRequest(http.MethodGet, "/backup", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusOK {
		t.Fatalf("GET /backup = %d (%s)", rec.Code, rec.Body.String())
	}
	var body map[string]any
	if err := json.Unmarshal(rec.Body.Bytes(), &body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if body["status"] == nil || body["snapshots"] == nil {
		t.Fatalf("bad body: %s", rec.Body.String())
	}
	// Password must never appear.
	if strings.Contains(rec.Body.String(), "password\"") && strings.Contains(rec.Body.String(), `"password_set":false`) {
		t.Fatal("password field leaked")
	}
}

// TestHandlerBadMethod: run on GET is 405.
func TestHandlerBadMethod(t *testing.T) {
	stateDir := t.TempDir()
	mux := http.NewServeMux()
	Register(mux, stateDir, nil)
	req := httptest.NewRequest(http.MethodGet, "/backup/run", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("GET /backup/run = %d, want 405", rec.Code)
	}
}

// TestCaptureTree: staging a stack dir + volume lands the
// ADR 0002 payload layout on disk (stack/ + volumes/<name>/).
func TestCaptureTree(t *testing.T) {
	stateDir := t.TempDir()
	stackDir := filepath.Join(stateDir, "demo")
	if err := os.MkdirAll(stackDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stackDir, "compose.yaml"), []byte("services: {}\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	volRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(volRoot, "hello.txt"), []byte("hi"), 0o644); err != nil {
		t.Fatal(err)
	}
	vols := &fakeBackend{volumes: map[string]string{"demo_data": volRoot}}
	dest := filepath.Join(t.TempDir(), "stage")
	if err := captureTree(context.Background(), stateDir, "demo", dest, vols); err != nil {
		t.Fatalf("captureTree: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "stack", "compose.yaml")); err != nil {
		t.Fatalf("stack file not staged: %v", err)
	}
	if b, err := os.ReadFile(filepath.Join(dest, "volumes", "data", "hello.txt")); err != nil || string(b) != "hi" {
		t.Fatalf("volume content not staged: %v %q", err, b)
	}
}

// TestFindStackPayload: restic nests the staged tree under its
// original tmpdir path — restore lands it at
// <target>/tmp/ferriage-backup-*/stack.
func TestFindStackPayload(t *testing.T) {
	root := t.TempDir()
	payload := filepath.Join(root, "tmp", "ferriage-backup-12345")
	if err := os.MkdirAll(filepath.Join(payload, "stack"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(payload, "volumes", "data"), 0o755); err != nil {
		t.Fatal(err)
	}
	if got := findStackPayload(root); got != payload {
		t.Errorf("findStackPayload = %q, want %q", got, payload)
	}
	roots, _ := findNodePayloads(filepath.Join(root, "tmp"))
	if len(roots) != 0 {
		t.Errorf("findNodePayloads on single-stack payload = %v, want 0", roots)
	}
}

// TestParseSnapshotID: restic "snapshot <id> saved" style lines.
func TestParseSnapshotID(t *testing.T) {
	out := "repository opened\nsnapshot abc123def456 saved\n"
	if got := parseSnapshotID(out); got != "abc123def456" {
		t.Errorf("parseSnapshotID = %q, want abc123def456", got)
	}
	if got := parseSnapshotID("no snapshot here"); got != "" {
		t.Errorf("parseSnapshotID(garbage) = %q, want empty", got)
	}
}
