// Package state manages the stacks directory: Compose files on disk are
// the source of truth (CONCEPT.md). This file covers the on-disk half of
// Phase 1.2 — ensuring the directory exists and listing the stack names
// recorded there. Adoption of running stacks happens in internal/adopt.
package state

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
)

// Dir is the stacks directory. Its subdirectory names are the stack names;
// the state-sync wire format is decided by ADR before Phase 1.3.
type Dir struct {
	Root string
}

// New returns a Dir rooted at root, creating it (with parents) if needed.
func New(root string) (*Dir, error) {
	if root == "" {
		root = "stacks"
	}
	if err := os.MkdirAll(root, 0o755); err != nil {
		return nil, fmt.Errorf("state: create stacks dir %s: %w", root, err)
	}
	return &Dir{Root: root}, nil
}

// List returns the names of stack directories under Root, sorted.
// A missing subdirectory (e.g. fresh install) yields an empty list.
func (d *Dir) List() ([]string, error) {
	entries, err := os.ReadDir(d.Root)
	if err != nil {
		return nil, fmt.Errorf("state: read stacks dir %s: %w", d.Root, err)
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		if e.IsDir() {
			names = append(names, e.Name())
		}
	}
	sort.Strings(names)
	return names, nil
}

// StackPath returns the path of the named stack's directory.
func (d *Dir) StackPath(name string) string {
	return filepath.Join(d.Root, name)
}

// EnsureStack creates the named stack's directory if it does not exist.
func (d *Dir) EnsureStack(name string) error {
	if err := os.MkdirAll(d.StackPath(name), 0o755); err != nil {
		return fmt.Errorf("state: create stack dir %s: %w", name, err)
	}
	return nil
}
