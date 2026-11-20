package state

import (
	"os"
	"path/filepath"
	"testing"
)

func TestNewListEmpty(t *testing.T) {
	root := filepath.Join(t.TempDir(), "stacks")
	d, err := New(root)
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if info, err := os.Stat(root); err != nil || !info.IsDir() {
		t.Fatalf("root not created: %v", err)
	}
	names, err := d.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 0 {
		t.Errorf("empty dir: got %v, want []", names)
	}
}

func TestEnsureStackAndList(t *testing.T) {
	d, err := New(t.TempDir())
	if err != nil {
		t.Fatalf("new: %v", err)
	}
	if err := d.EnsureStack("beta"); err != nil {
		t.Fatalf("ensure beta: %v", err)
	}
	if err := d.EnsureStack("alpha"); err != nil {
		t.Fatalf("ensure alpha: %v", err)
	}
	// A regular file is not a stack.
	if err := os.WriteFile(filepath.Join(d.Root, "README"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write file: %v", err)
	}
	names, err := d.List()
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(names) != 2 || names[0] != "alpha" || names[1] != "beta" {
		t.Errorf("list = %v, want [alpha beta]", names)
	}
	if got := d.StackPath("alpha"); got != filepath.Join(d.Root, "alpha") {
		t.Errorf("StackPath = %q", got)
	}
}
