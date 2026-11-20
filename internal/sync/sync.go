// Package sync implements the state-sync wire protocol of ADR 0001:
// a sha256 manifest of the stacks directory plus blob fetch, reconciled
// last-writer-wins per file with a durable tombstone ledger so a
// remote delete is honored (never healed back from a stale peer) and a
// brand-new peer never deletes on its first sync.
package sync

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/NopeNix/Ferriage/internal/identity"
)

// MaxBlobBytes caps a single blob (default policy in ADR 0001).
const MaxBlobBytes = 64 << 20

// Entry is one file in the manifest.
type Entry struct {
	Path      string    `json:"path"`
	SHA256    string    `json:"sha256"`
	Size      int64     `json:"size"`
	MTime     time.Time `json:"mtime"`
	UpdatedAt time.Time `json:"updated_at"`
	PeerID    string    `json:"peer_id"`
}

// Manifest is the wire body of GET /sync/manifest.
type Manifest struct {
	PeerID      string    `json:"peer_id"`
	GeneratedAt time.Time `json:"generated_at"`
	Entries     []Entry   `json:"entries"`
}

// Error is returned by the store for HTTP-mapped failures.
type Error struct {
	Code string
	Msg  string
}

func (e *Error) Error() string { return e.Msg }

func notFound(msg string) error { return &Error{Code: "not_found", Msg: msg} }

// Store reads and writes the synced state tree.
type Store struct {
	Root   string
	PeerID string
}

// New returns a Store rooted at root (the state dir). root's
// .ferriage subdirectory is never walked.
func New(root, peerID string) *Store {
	return &Store{Root: root, PeerID: peerID}
}

// ValidatePath rejects anything that is not a clean, relative,
// slash-separated path inside the state tree.
func ValidatePath(p string) error {
	if p == "" || strings.HasPrefix(p, "/") || strings.HasPrefix(p, `\`) {
		return &Error{Code: "bad_path", Msg: "invalid path"}
	}
	clean := filepath.ToSlash(filepath.Clean(p))
	if clean == "." || clean != p {
		return &Error{Code: "bad_path", Msg: "invalid path"}
	}
	for _, seg := range strings.Split(clean, "/") {
		if seg == "" || seg == "." || seg == ".." || seg == identity.DirName {
			return &Error{Code: "bad_path", Msg: "invalid path"}
		}
	}
	return nil
}

// Resolve returns the absolute on-disk path for a validated manifest
// path, verifying no symlink escapes Root.
func (s *Store) Resolve(p string) (string, error) {
	if err := ValidatePath(p); err != nil {
		return "", err
	}
	abs := filepath.Join(s.Root, filepath.FromSlash(p))
	// A symlink inside the tree must never let a sync write outside it.
	rootAbs, err := filepath.Abs(s.Root)
	if err != nil {
		return "", err
	}
	// Walk up to the deepest existing ancestor and check it stays inside.
	probe := abs
	for {
		if _, err := os.Lstat(probe); err == nil {
			break
		}
		parent := filepath.Dir(probe)
		if parent == probe {
			break
		}
		probe = parent
	}
	real, err := filepath.EvalSymlinks(probe)
	if err != nil {
		return "", err
	}
	if real != rootAbs && !strings.HasPrefix(real, rootAbs+string(os.PathSeparator)) {
		return "", &Error{Code: "bad_path", Msg: "path escapes state root"}
	}
	return abs, nil
}

// BuildManifest walks the state tree and returns a manifest rebuilt
// from the actual files on disk.
func (s *Store) BuildManifest() (*Manifest, error) {
	var entries []Entry
	err := filepath.WalkDir(s.Root, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return err
		}
		rel, err := filepath.Rel(s.Root, path)
		if err != nil {
			return err
		}
		rel = filepath.ToSlash(rel)
		if rel == identity.DirName || strings.HasPrefix(rel, identity.DirName+"/") {
			if d.IsDir() {
				return filepath.SkipDir
			}
			return nil
		}
		if d.IsDir() || d.Type()&os.ModeSymlink != 0 {
			return nil
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		sum, err := sha256File(path)
		if err != nil {
			return err
		}
		entries = append(entries, Entry{
			Path:      rel,
			SHA256:    sum,
			Size:      info.Size(),
			MTime:     info.ModTime(),
			UpdatedAt: info.ModTime(),
			PeerID:    s.PeerID,
		})
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("sync: walk: %w", err)
	}
	sort.Slice(entries, func(i, j int) bool { return entries[i].Path < entries[j].Path })
	if entries == nil {
		entries = []Entry{}
	}
	return &Manifest{PeerID: s.PeerID, GeneratedAt: time.Now().UTC(), Entries: entries}, nil
}

func sha256File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer func() { _ = f.Close() }()
	h := sha256.New()
	if _, err := f.WriteTo(h); err != nil {
		return "", err
	}
	return hex.EncodeToString(h.Sum(nil)), nil
}

// ReadBlob returns the exact bytes of a validated path.
func (s *Store) ReadBlob(p string) ([]byte, error) {
	abs, err := s.Resolve(p)
	if err != nil {
		return nil, err
	}
	info, err := os.Stat(abs)
	if err != nil {
		if os.IsNotExist(err) {
			return nil, notFound("blob not found")
		}
		return nil, err
	}
	if info.IsDir() {
		return nil, notFound("blob not found")
	}
	if info.Size() > MaxBlobBytes {
		return nil, &Error{Code: "too_large", Msg: "blob exceeds size cap"}
	}
	data, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}
	if int64(len(data)) > MaxBlobBytes {
		return nil, &Error{Code: "too_large", Msg: "blob exceeds size cap"}
	}
	return data, nil
}

// AtomicWrite writes data to a validated path via a temp file in the
// same directory, verifying the sha256 sum before the rename so a
// corrupt transfer can never replace a good file. Parents are created.
func (s *Store) AtomicWrite(p string, data []byte, wantSHA string) error {
	abs, err := s.Resolve(p)
	if err != nil {
		return err
	}
	got := sha256.Sum256(data)
	if hex.EncodeToString(got[:]) != wantSHA {
		return &Error{Code: "hash_mismatch", Msg: "sha256 mismatch"}
	}
	dir := filepath.Dir(abs)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(dir, ".ferriage-tmp-*")
	if err != nil {
		return err
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		_ = os.Remove(tmpName)
		return err
	}
	if err := tmp.Close(); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	if err := os.Rename(tmpName, abs); err != nil {
		_ = os.Remove(tmpName)
		return err
	}
	return nil
}

// Remove deletes a validated path if present. Removing a stack dir
// removes its (then possibly empty) parent dirs.
func (s *Store) Remove(p string) error {
	abs, err := s.Resolve(p)
	if err != nil {
		return err
	}
	if err := os.Remove(abs); err != nil && !os.IsNotExist(err) {
		return err
	}
	dir := filepath.Dir(abs)
	for dir != s.Root && strings.HasPrefix(dir, s.Root) {
		if err := os.Remove(dir); err != nil {
			break
		}
		dir = filepath.Dir(dir)
	}
	return nil
}

// LedgerEntry records what a pull last applied for one path.
type LedgerEntry struct {
	UpdatedAt time.Time `json:"updated_at"`
	PeerID    string    `json:"peer_id"`
	SHA256    string    `json:"sha256"`
	Deleted   bool      `json:"deleted"`
}

// Ledger is the durable per-peer provenance/tombstone record.
type Ledger struct {
	Entries map[string]LedgerEntry `json:"entries"`
	path    string
}

// LoadLedger reads the sync ledger, or returns an empty one.
func LoadLedger(stateRoot string) (*Ledger, error) {
	path := filepath.Join(identity.Dir(stateRoot), "sync-ledger.json")
	l := &Ledger{Entries: map[string]LedgerEntry{}, path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return l, nil
		}
		return nil, fmt.Errorf("sync: read ledger: %w", err)
	}
	if err := json.Unmarshal(data, l); err != nil {
		return nil, fmt.Errorf("sync: decode ledger: %w", err)
	}
	if l.Entries == nil {
		l.Entries = map[string]LedgerEntry{}
	}
	return l, nil
}

// HasSynced reports whether this peer has ever completed a pull.
func (l *Ledger) HasSynced() bool {
	for _, e := range l.Entries {
		if !e.Deleted {
			return true
		}
	}
	return false
}

// Save persists the ledger atomically.
func (l *Ledger) Save() error {
	if err := os.MkdirAll(filepath.Dir(l.path), 0o755); err != nil {
		return err
	}
	data, err := json.MarshalIndent(l, "", "  ")
	if err != nil {
		return err
	}
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}
