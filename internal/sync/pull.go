package sync

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"strings"
	"time"
)

// Fetcher gets a manifest and a blob from one peer.
type Fetcher interface {
	GetManifest(ctx context.Context, base string) (*Manifest, error)
	GetBlob(ctx context.Context, base, path string) ([]byte, error)
}

// HTTPFetcher is the plain-HTTP carrier of ADR 0001 (Phase 1).
type HTTPFetcher struct {
	Client *http.Client
}

func (f *HTTPFetcher) GetManifest(ctx context.Context, base string) (*Manifest, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(base, "/")+"/sync/manifest", nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sync: manifest %s: %s", base, resp.Status)
	}
	var m Manifest
	if err := json.NewDecoder(io.LimitReader(resp.Body, 32<<20)).Decode(&m); err != nil {
		return nil, fmt.Errorf("sync: decode manifest: %w", err)
	}
	return &m, nil
}

func (f *HTTPFetcher) GetBlob(ctx context.Context, base, path string) ([]byte, error) {
	u := strings.TrimSuffix(base, "/") + "/sync/blob?path=" + url.QueryEscape(path)
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u, nil)
	if err != nil {
		return nil, err
	}
	resp, err := f.Client.Do(req)
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("sync: blob %s: %s", path, resp.Status)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBlobBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > MaxBlobBytes {
		return nil, fmt.Errorf("sync: blob %s exceeds size cap", path)
	}
	return data, nil
}

// newer reports whether remote entry r should replace local entry l
// under (updated_at, peer_id) LWW, SHA breaking remaining ties.
func newer(r, l Entry) bool {
	if !r.UpdatedAt.Equal(l.UpdatedAt) {
		return r.UpdatedAt.After(l.UpdatedAt)
	}
	if r.PeerID != l.PeerID {
		return r.PeerID > l.PeerID
	}
	return r.SHA256 > l.SHA256
}

// Apply reconciles one fetched manifest into the local store under the
// tombstone rules of ADR 0001. ledger records what was applied.
func Apply(ctx context.Context, store *Store, ledger *Ledger, fetch Fetcher, base string, m *Manifest) error {
	local, err := store.BuildManifest()
	if err != nil {
		return err
	}
	localByPath := map[string]Entry{}
	for _, e := range local.Entries {
		localByPath[e.Path] = e
	}
	remoteByPath := map[string]Entry{}
	for _, e := range m.Entries {
		remoteByPath[e.Path] = e
	}

	// Pull: fetch blobs for remote entries that are newer and differ.
	for _, r := range m.Entries {
		l, ok := localByPath[r.Path]
		if !ok {
			if le, has := ledger.Entries[r.Path]; has && le.Deleted && !newer(r, Entry{UpdatedAt: le.UpdatedAt, PeerID: le.PeerID, SHA256: le.SHA256}) {
				continue // honored tombstone: not healed back
			}
			if err := pullOne(ctx, store, fetch, base, r); err != nil {
				return err
			}
			ledger.Entries[r.Path] = LedgerEntry{UpdatedAt: localMTime(store, r.Path), PeerID: r.PeerID, SHA256: r.SHA256}
			continue
		}
		if r.SHA256 == l.SHA256 {
			continue
		}
		if !newer(r, l) {
			continue // local write is newer; keep it, converge later
		}
		if err := pullOne(ctx, store, fetch, base, r); err != nil {
			return err
		}
		ledger.Entries[r.Path] = LedgerEntry{UpdatedAt: localMTime(store, r.Path), PeerID: r.PeerID, SHA256: r.SHA256}
	}

	// Delete propagation: a local path absent from the remote manifest
	// is deleted only if the ledger knows it as a synced copy AND the
	// user has not rewritten it since. A brand-new peer (empty ledger)
	// therefore never deletes on its first sync — satisfying ADR 0001
	// without any "first sync" flag. A path the user edited after the
	// last pull is kept (its mtime is newer than the ledger entry); a
	// synced copy that is untouched since is deleted.
	for _, l := range local.Entries {
		if _, ok := remoteByPath[l.Path]; ok {
			continue
		}
		le, ok := ledger.Entries[l.Path]
		if !ok {
			continue // never synced (or brand-new peer): keep local work
		}
		if l.MTime.After(le.UpdatedAt) {
			continue // edited locally since the pull: keep
		}
		if err := store.Remove(l.Path); err != nil {
			return err
		}
		ledger.Entries[l.Path] = LedgerEntry{Deleted: true, UpdatedAt: m.GeneratedAt, PeerID: m.PeerID}
	}

	// Refresh ledger entries for files we already had in sync so a
	// later user edit is detected via mtime. Use the local entry's
	// mtime (the actual file on disk) as the baseline: the remote
	// entry's updated_at is a different clock's value and could be
	// newer than any local mtime, which would suppress deletes.
	for _, l := range local.Entries {
		if _, ok := remoteByPath[l.Path]; !ok {
			continue
		}
		if _, ok := ledger.Entries[l.Path]; !ok {
			ledger.Entries[l.Path] = LedgerEntry{UpdatedAt: l.MTime, PeerID: l.PeerID, SHA256: l.SHA256}
		}
	}
	return ledger.Save()
}

// localMTime returns the actual mtime of a just-pulled file on disk —
// the correct "user edited since" baseline for the delete check.
func localMTime(store *Store, p string) time.Time {
	abs, err := store.Resolve(p)
	if err != nil {
		return time.Time{}
	}
	info, err := os.Stat(abs)
	if err != nil {
		return time.Time{}
	}
	return info.ModTime()
}

func pullOne(ctx context.Context, store *Store, fetch Fetcher, base string, r Entry) error {
	data, err := fetch.GetBlob(ctx, base, r.Path)
	if err != nil {
		return err
	}
	return store.AtomicWrite(r.Path, data, r.SHA256)
}

// SyncOnce pulls once from every configured peer.
func SyncOnce(ctx context.Context, store *Store, ledger *Ledger, fetch Fetcher, peers []string) error {
	for _, base := range peers {
		m, err := fetch.GetManifest(ctx, base)
		if err != nil {
			return fmt.Errorf("sync: peer %s: %w", base, err)
		}
		if err := Apply(ctx, store, ledger, fetch, base, m); err != nil {
			return fmt.Errorf("sync: apply %s: %w", base, err)
		}
	}
	return nil
}

// Loop pulls from the configured peers immediately on start, then on
// every tick, until ctx ends. A single goroutine owns the ledger, so
// no locking is needed.
func Loop(ctx context.Context, store *Store, ledger *Ledger, fetch Fetcher, peers []string, interval time.Duration) {
	if interval <= 0 {
		interval = 30 * time.Second
	}
	_ = SyncOnce(ctx, store, ledger, fetch, peers)
	t := time.NewTicker(interval)
	defer t.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-t.C:
			if err := SyncOnce(ctx, store, ledger, fetch, peers); err != nil {
				// Log-and-continue: a peer being down is normal.
				// Caller passes a logger via ctx is overkill; keep
				// the loop silent and let /healthz+UI surface it.
				_ = err
			}
		}
	}
}
