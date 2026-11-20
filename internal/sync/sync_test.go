package sync

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func sha(b []byte) string { sum := sha256.Sum256(b); return hex.EncodeToString(sum[:]) }

func writeFile(t *testing.T, root, p string, data []byte) {
	t.Helper()
	full := filepath.Join(root, filepath.FromSlash(p))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, data, 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestValidatePath(t *testing.T) {
	ok := []string{"a/compose.yaml", "a/b/c.txt", "x"}
	for _, p := range ok {
		if err := ValidatePath(p); err != nil {
			t.Errorf("ValidatePath(%q) = %v, want nil", p, err)
		}
	}
	bad := []string{"", "../x", "a/../../x", "/abs", "a//b", "a/./b", ".", ".ferriage/identity.json", "a/.ferriage/x"}
	for _, p := range bad {
		if err := ValidatePath(p); err == nil {
			t.Errorf("ValidatePath(%q) = nil, want error", p)
		}
	}
}

func TestManifestSkipsFerriageAndSymlinks(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "web/compose.yaml", []byte("services: {}\n"))
	writeFile(t, root, ".ferriage/identity.json", []byte("{}"))
	writeFile(t, root, ".ferriage/sync-ledger.json", []byte("{}"))
	// A symlink inside the tree — never followed.
	if err := os.Symlink(filepath.Join(root, "web"), filepath.Join(root, "linkdir")); err != nil {
		t.Skipf("symlink unsupported: %v", err)
	}
	s := New(root, "peer-a")
	m, err := s.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range m.Entries {
		if e.Path != "web/compose.yaml" {
			t.Errorf("manifest contains unexpected %q", e.Path)
		}
	}
	if want := sha([]byte("services: {}\n")); len(m.Entries) != 1 || m.Entries[0].SHA256 != want {
		t.Errorf("manifest = %+v, want one entry with sha %s", m.Entries, want)
	}
}

func TestAtomicWriteVerifiesHash(t *testing.T) {
	root := t.TempDir()
	s := New(root, "peer-a")
	data := []byte("hello")
	if err := s.AtomicWrite("a/b.txt", data, "deadbeef"); err == nil {
		t.Fatal("mismatched hash accepted")
	}
	if _, err := os.Stat(filepath.Join(root, "a", "b.txt")); !os.IsNotExist(err) {
		t.Fatalf("bad write left a file behind: %v", err)
	}
	if err := s.AtomicWrite("a/b.txt", data, sha(data)); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(root, "a", "b.txt"))
	if err != nil || string(got) != "hello" {
		t.Fatalf("content = %q, %v", got, err)
	}
}

type memFetcher struct{ blobs map[string][]byte }

func (f memFetcher) GetManifest(ctx context.Context, base string) (*Manifest, error) {
	return nil, nil
}
func (f memFetcher) GetBlob(ctx context.Context, base, path string) ([]byte, error) {
	data, ok := f.blobs[path]
	if !ok {
		return nil, notFound("blob not found")
	}
	return data, nil
}

func TestApplyPullsNewerAndDeletesTombstoned(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()
	writeFile(t, src, "web/compose.yaml", []byte("v2"))
	s := New(src, "peer-b")
	m, err := s.BuildManifest()
	if err != nil {
		t.Fatal(err)
	}
	fetch := memFetcher{blobs: map[string][]byte{"web/compose.yaml": []byte("v2")}}
	dstStore := New(dst, "peer-a")
	ledger, err := LoadLedger(dst)
	if err != nil {
		t.Fatal(err)
	}
	if err := Apply(context.Background(), dstStore, ledger, fetch, "http://b", m); err != nil {
		t.Fatal(err)
	}
	got, err := os.ReadFile(filepath.Join(dst, "web", "compose.yaml"))
	if err != nil || string(got) != "v2" {
		t.Fatalf("after pull: %q, %v", got, err)
	}

	// Remote deletes the file; the next manifest is empty.
	m2 := &Manifest{PeerID: "peer-b", GeneratedAt: time.Now().UTC(), Entries: []Entry{}}
	if err := Apply(context.Background(), dstStore, ledger, fetch, "http://b", m2); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "web", "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("tombstoned file still present: %v", err)
	}
	// The tombstone is honored: a stale peer offering the old content
	// must not resurrect it.
	stale := &Manifest{PeerID: "peer-b", GeneratedAt: time.Now().UTC(), Entries: []Entry{
		{Path: "web/compose.yaml", SHA256: sha([]byte("v1")), UpdatedAt: time.Now().Add(-time.Hour)},
	}}
	if err := Apply(context.Background(), dstStore, ledger, fetch, "http://b", stale); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dst, "web", "compose.yaml")); !os.IsNotExist(err) {
		t.Fatalf("tombstone healed back by stale manifest: %v", err)
	}
}

func TestBrandNewPeerNeverDeletes(t *testing.T) {
	root := t.TempDir()
	writeFile(t, root, "local/compose.yaml", []byte("mine"))
	ledger, err := LoadLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	empty := &Manifest{PeerID: "peer-b", GeneratedAt: time.Now().UTC(), Entries: []Entry{}}
	fetch := memFetcher{blobs: map[string][]byte{}}
	if err := Apply(context.Background(), New(root, "peer-a"), ledger, fetch, "http://b", empty); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(root, "local", "compose.yaml")); err != nil {
		t.Fatalf("brand-new peer deleted local file: %v", err)
	}
}

func TestNewerLWW(t *testing.T) {
	older := Entry{UpdatedAt: time.Unix(100, 0), PeerID: "peer-a", SHA256: "a"}
	newerE := Entry{UpdatedAt: time.Unix(200, 0), PeerID: "peer-a", SHA256: "b"}
	if !newer(newerE, older) || newer(older, newerE) {
		t.Fatal("updated_at ordering wrong")
	}
	sameTime := Entry{UpdatedAt: time.Unix(100, 0), PeerID: "peer-b", SHA256: "a"}
	if !newer(sameTime, older) {
		t.Fatal("peer_id tie-break wrong")
	}
	samePeer := Entry{UpdatedAt: time.Unix(100, 0), PeerID: "peer-a", SHA256: "z"}
	if !newer(samePeer, older) {
		t.Fatal("sha tie-break wrong")
	}
}
