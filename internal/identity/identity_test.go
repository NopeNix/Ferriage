package identity

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadOrCreateRoundTrip(t *testing.T) {
	root := t.TempDir()
	a, err := LoadOrCreate(root)
	if err != nil {
		t.Fatalf("create: %v", err)
	}
	if len(a.ID()) != 64 {
		t.Errorf("id len = %d, want 64 hex chars", len(a.ID()))
	}
	b, err := LoadOrCreate(root)
	if err != nil {
		t.Fatalf("reload: %v", err)
	}
	if a.ID() != b.ID() {
		t.Errorf("id changed: %q vs %q", a.ID(), b.ID())
	}
	if !b.Public.Equal(a.Public) {
		t.Errorf("public key changed after reload")
	}
}

func TestIdentityFilePermissionsAndLocation(t *testing.T) {
	root := t.TempDir()
	if _, err := LoadOrCreate(root); err != nil {
		t.Fatalf("create: %v", err)
	}
	path := filepath.Join(root, DirName, "identity.json")
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat: %v", err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("identity.json perm = %o, want 600", info.Mode().Perm())
	}
}
