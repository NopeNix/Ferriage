package sync_test

import (
	"context"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NopeNix/Ferriage/internal/sync"
)

// Two real HTTP peers syncing file content over the wire (ADR 0001).
func TestTwoPeersSyncOverHTTP(t *testing.T) {
	rootA, rootB := t.TempDir(), t.TempDir()
	storeA := sync.New(rootA, "peer-a")
	storeB := sync.New(rootB, "peer-b")
	srvA := httptest.NewServer(sync.NewHandler(storeA, "peer-a"))
	srvB := httptest.NewServer(sync.NewHandler(storeB, "peer-b"))
	defer srvA.Close()
	defer srvB.Close()

	fetch := &sync.HTTPFetcher{Client: srvA.Client()}
	ledgerA, err := sync.LoadLedger(rootA)
	if err != nil {
		t.Fatal(err)
	}
	ledgerB, err := sync.LoadLedger(rootB)
	if err != nil {
		t.Fatal(err)
	}

	write := func(root, p string, data []byte) {
		t.Helper()
		full := filepath.Join(root, filepath.FromSlash(p))
		if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(full, data, 0o644); err != nil {
			t.Fatal(err)
		}
		if err := os.Chtimes(full, time.Now(), time.Now()); err != nil {
			t.Fatal(err)
		}
	}

	// A creates a stack; B pulls it.
	write(rootA, "web/compose.yaml", []byte("services:\n  web:\n    image: nginx\n"))
	if err := sync.SyncOnce(context.Background(), storeB, ledgerB, fetch, []string{srvA.URL}); err != nil {
		t.Fatalf("B pulls A: %v", err)
	}
	got, err := os.ReadFile(filepath.Join(rootB, "web", "compose.yaml"))
	if err != nil || string(got) != "services:\n  web:\n    image: nginx\n" {
		t.Fatalf("B content = %q, %v", got, err)
	}

	// B edits it with a newer mtime; A pulls the edit.
	time.Sleep(10 * time.Millisecond)
	write(rootB, "web/compose.yaml", []byte("services:\n  web:\n    image: caddy\n"))
	if err := sync.SyncOnce(context.Background(), storeA, ledgerA, fetch, []string{srvB.URL}); err != nil {
		t.Fatalf("A pulls B: %v", err)
	}
	got, err = os.ReadFile(filepath.Join(rootA, "web", "compose.yaml"))
	if err != nil || string(got) != "services:\n  web:\n    image: caddy\n" {
		t.Fatalf("A content = %q, %v", got, err)
	}

	// B deletes it; A honors the tombstone.
	time.Sleep(10 * time.Millisecond)
	if err := os.Remove(filepath.Join(rootB, "web", "compose.yaml")); err != nil {
		t.Fatal(err)
	}
	if err := sync.SyncOnce(context.Background(), storeA, ledgerA, fetch, []string{srvB.URL}); err != nil {
		t.Fatalf("A pulls B delete: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootA, "web", "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("delete not honored: %v", err)
	}

	// The other direction: A creates a file, B gets it on next pull.
	write(rootA, "db/compose.yaml", []byte("services:\n  db:\n    image: postgres\n"))
	if err := sync.SyncOnce(context.Background(), storeB, ledgerB, fetch, []string{srvA.URL}); err != nil {
		t.Fatalf("B pulls A again: %v", err)
	}
	if _, err := os.Stat(filepath.Join(rootB, "db", "compose.yaml")); err != nil {
		t.Fatalf("B missing new file: %v", err)
	}
}
