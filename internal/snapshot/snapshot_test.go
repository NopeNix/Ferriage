package snapshot

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/docker/docker/api/types/volume"
)

// fakeBackend fakes the daemon's volume surface with an in-memory
// map of name -> mountpoint (a temp dir on disk).
type fakeBackend struct {
	volumes map[string]string // name -> mountpoint
	labels  map[string]string
}

func (f *fakeBackend) VolumeList(ctx context.Context, opts volume.ListOptions) (volume.ListResponse, error) {
	var out []*volume.Volume
	for name, mp := range f.volumes {
		out = append(out, &volume.Volume{
			Name:       name,
			Mountpoint: mp,
			Labels:     map[string]string{"com.docker.compose.project": f.labels[name]},
		})
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
	if err := os.MkdirAll(mp, 0o755); err != nil {
		return volume.Volume{}, err
	}
	f.volumes[opts.Name] = mp
	return volume.Volume{Name: opts.Name, Mountpoint: mp}, nil
}

func (f *fakeBackend) VolumeRemove(ctx context.Context, id string, force bool) error {
	mp, ok := f.volumes[id]
	if !ok {
		return fmt.Errorf("no such volume")
	}
	_ = os.RemoveAll(mp)
	delete(f.volumes, id)
	return nil
}

// VolumeCopyOut tars the volume dir (mountpoint) to w, GNU-tar on the
// host, mirroring the daemon's CopyFromContainer.
func (f *fakeBackend) VolumeCopyOut(ctx context.Context, volumeName string, w io.Writer) error {
	dir, ok := f.volumes[volumeName]
	if !ok {
		return fmt.Errorf("no such volume")
	}
	cmd := exec.CommandContext(ctx, "tar", "-cf", "-", "-C", dir, ".")
	cmd.Stdout = w
	return cmd.Run()
}

// VolumeCopyIn untars stdin into the volume dir, mirroring the
// daemon's CopyToContainer.
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

func writeTestStack(t *testing.T, stateDir, name string) {
	t.Helper()
	dir := filepath.Join(stateDir, name)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), []byte("services:\n  app:\n    image: x\n"), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestExportRestoreRoundTrip(t *testing.T) {
	stateDir := t.TempDir()
	writeTestStack(t, stateDir, "demo")
	volRoot := t.TempDir()
	if err := os.WriteFile(filepath.Join(volRoot, "data.txt"), []byte("hello volume"), 0o644); err != nil {
		t.Fatal(err)
	}
	backend := &fakeBackend{volumes: map[string]string{"demo_data": volRoot}, labels: map[string]string{"demo_data": "demo"}}

	// Export to a buffer.
	var buf bytes.Buffer
	if err := Export(context.Background(), &buf, stateDir, "demo", "peer-1", backend); err != nil {
		t.Fatalf("Export: %v", err)
	}
	if buf.Len() == 0 {
		t.Fatal("empty archive")
	}

	// Remove the source volume (the destination daemon must not have
	// demo_data already), then restore into a fresh state dir.
	if err := backend.VolumeRemove(context.Background(), "demo_data", true); err != nil {
		t.Fatal(err)
	}
	fresh := t.TempDir()
	m, err := Restore(context.Background(), bytes.NewReader(buf.Bytes()), fresh, "demo", backend)
	if err != nil {
		t.Fatalf("Restore: %v", err)
	}
	if m.Stack != "demo" || len(m.Volumes) != 1 || m.Volumes[0].Name != "data" {
		t.Errorf("manifest = %+v", m)
	}
	data, err := os.ReadFile(filepath.Join(fresh, "demo", "compose.yaml"))
	if err != nil || string(data) != "services:\n  app:\n    image: x\n" {
		t.Errorf("compose.yaml = %q, %v", data, err)
	}
	mp := backend.volumes["demo_data"]
	restored, err := os.ReadFile(filepath.Join(mp, "data.txt"))
	if err != nil || string(restored) != "hello volume" {
		t.Errorf("restored volume data = %q, %v (mp=%s)", restored, err, mp)
	}
}

// craftArchive builds a gzip+tar archive with the given manifest body
// and a single data entry, for hostile-archive tests.
func craftArchive(t *testing.T, manifest string) []byte {
	t.Helper()
	var buf bytes.Buffer
	gz := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gz)
	if err := tw.WriteHeader(&tar.Header{Name: "manifest.json", Mode: 0o644, Size: int64(len(manifest))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write([]byte(manifest)); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}
	if err := gz.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func TestExportMissingStack(t *testing.T) {
	err := Export(context.Background(), &bytes.Buffer{}, t.TempDir(), "nope", "peer", &fakeBackend{volumes: map[string]string{}})
	if err == nil || !strings.Contains(err.Error(), "not found") {
		t.Fatalf("err = %v, want not found", err)
	}
}

func TestRestoreRefusesExistingStack(t *testing.T) {
	stateDir := t.TempDir()
	writeTestStack(t, stateDir, "demo")
	backend := &fakeBackend{volumes: map[string]string{}}
	var buf bytes.Buffer
	if err := Export(context.Background(), &buf, stateDir, "demo", "peer", backend); err != nil {
		t.Fatalf("Export: %v", err)
	}
	// State dir already has demo/ -> restore must refuse.
	if _, err := Restore(context.Background(), bytes.NewReader(buf.Bytes()), stateDir, "demo", backend); err == nil {
		t.Fatal("expected refusal for existing stack")
	}
}

func TestRestoreRejectsTraversal(t *testing.T) {
	// hand() builds an archive whose entry carries a path escape.
	stateDir := t.TempDir()
	backend := &fakeBackend{volumes: map[string]string{}}
	// Build archive with a malicious entry name via the manifest listing
	// trick: use Export output but doctor the bytes is hard; instead
	// craft directly.
	raw := craftArchive(t, `{"format_version":1,"stack":"x","files":["stack/../../evil"]}`)
	if _, err := Restore(context.Background(), bytes.NewReader(raw), stateDir, "x", backend); err == nil {
		t.Fatal("expected traversal rejection")
	}
	if _, err := os.Stat(filepath.Join(stateDir, "..", "evil")); err == nil {
		t.Fatal("escape happened")
	}
}
