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
// tombstone rules of ADR 0001 (as amended by ADR 0012). ledger records
// what was applied, including each path's lineage peer.
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

	// Local deletes become tombstones FIRST. When the operator
	// removes a stack dir out-of-band (rm — there is no delete
	// API; the state dir IS the API), the peer must record the
	// deletion in its own ledger before pulling anything.
	// Otherwise it re-pulls the file from a mesh holder on this
	// very round and heals its own delete back forever, so the
	// delete never propagates (found by the 3-peer layer-2.5
	// delete test: the originator kept resurrecting the stack
	// from peer B). Only ledger-known synced paths qualify —
	// never-synced local work is untouched.
	for path, le := range ledger.Entries {
		if le.Deleted {
			continue
		}
		if _, ok := localByPath[path]; ok {
			continue // still on disk: possibly edited, not deleted
		}
		ledger.Entries[path] = LedgerEntry{
			Deleted:   true,
			UpdatedAt: le.UpdatedAt,
			PeerID:    le.PeerID,
			OriginID:  le.OriginID,
			SHA256:    le.SHA256,
		}
	}

	// Pull: fetch blobs for remote entries that are newer and differ.
	// Every successful pull records the entry's true origin — the
	// peer that last wrote the content, not the peer serving it —
	// so copies can never masquerade as originals (ADR 0012).
	for _, r := range m.Entries {
		l, ok := localByPath[r.Path]
		if !ok {
			if le, has := ledger.Entries[r.Path]; has && le.Deleted {
				// The tombstone honors the exact content it
				// deleted (same SHA). A different, newer write
				// revives the path: that is a genuine local
				// rewrite, not a stale re-offer.
				if r.SHA256 == le.SHA256 {
					continue // honored tombstone: not healed back
				}
				if !newer(r, Entry{UpdatedAt: le.UpdatedAt, PeerID: r.PeerID, SHA256: le.SHA256}) {
					continue // stale: not newer than the delete
				}
			}
			if err := pullOne(ctx, store, fetch, base, r); err != nil {
				return err
			}
			ledger.Entries[r.Path] = LedgerEntry{UpdatedAt: r.UpdatedAt, PeerID: r.PeerID, OriginID: originOfEntry(r, m.PeerID), SHA256: r.SHA256}
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
		ledger.Entries[r.Path] = LedgerEntry{UpdatedAt: r.UpdatedAt, PeerID: r.PeerID, OriginID: originOfEntry(r, m.PeerID), SHA256: r.SHA256}
	}

	// Delete propagation: a local path absent from the remote manifest
	// is deleted only if the ledger knows it as a synced copy AND the
	// user has not rewritten it since. A brand-new peer (empty ledger)
	// therefore never deletes on its first sync — satisfying ADR 0001
	// without any "first sync" flag. A path the user edited after the
	// last pull is kept (its mtime is newer than the ledger entry); a
	// synced copy that is untouched since is deleted.
	//
	// Multi-source rule (ADR 0012): the deleting peer must be the
	// path's ORIGIN peer — the peer that last wrote the content.
	// Absence from any other peer's manifest means "this source
	// never had it", which is what makes fan-in (one peer pulling
	// from two) safe. The origin is carried in the manifest
	// (Entry.OriginID) and pinned in the ledger, so a copy
	// served by a non-origin holder can never masquerade as an
	// original and block the originator's delete. Two earlier
	// attempts failed here, each caught by the next test layer:
	// per-source unanimity votes deadlock once every peer holds
	// a copy (each holder votes "present" forever), and
	// last-server lineage flips whenever a holder's local mtime
	// drift makes its copy "newer" than the originator's.
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
		origin := le.OriginID
		if origin == "" {
			origin = le.PeerID // pre-origin ledger: server == origin
		}
		if m.PeerID != origin {
			continue // not the origin peer: absence is not a delete
		}
		if err := store.Remove(l.Path); err != nil {
			return err
		}
		// Tombstone keeps the deleted entry's own lineage
		// (prev UpdatedAt/OriginID/SHA), not the manifest's
		// generation time: a stale re-offer of the same bytes
		// (same SHA) is honored-dead, while a genuinely newer
		// write revives the path.
		ledger.Entries[l.Path] = LedgerEntry{
			Deleted:   true,
			UpdatedAt: le.UpdatedAt,
			PeerID:    le.PeerID,
			OriginID:  le.OriginID,
			SHA256:    le.SHA256,
		}
	}

	// Refresh ledger entries for local files the ledger has
	// never seen — including purely local work — so a later
	// out-of-band delete of the originator's own file can
	// become a tombstone (see the scan above). Use the local
	// entry's mtime (the actual file on disk) as the baseline:
	// the remote entry's updated_at is a different clock's
	// value and could be newer than any local mtime, which
	// would suppress deletes. The local manifest's OriginID
	// is this peer for local writes, so the entry correctly
	// claims self-origin.
	for _, l := range local.Entries {
		if _, ok := ledger.Entries[l.Path]; !ok {
			ledger.Entries[l.Path] = LedgerEntry{UpdatedAt: l.MTime, PeerID: l.PeerID, OriginID: l.OriginID, SHA256: l.SHA256}
		}
	}
	return ledger.Save()
}

// originOfEntry resolves an entry's true origin: the manifest's
// OriginID when the serving peer reported one, else the serving
// peer (pre-origin manifests).
func originOfEntry(e Entry, server string) string {
	if e.OriginID != "" {
		return e.OriginID
	}
	return server
}

// pullOne fetches a blob and writes it, preserving the origin
// write-time as the file's mtime: copies then build identical
// manifests to the originator (no mtime drift), so LWW ties are
// broken by (updated_at, peer_id, sha) deterministically across
// the mesh instead of by pull order.
func pullOne(ctx context.Context, store *Store, fetch Fetcher, base string, r Entry) error {
	data, err := fetch.GetBlob(ctx, base, r.Path)
	if err != nil {
		return err
	}
	if err := store.AtomicWrite(r.Path, data, r.SHA256); err != nil {
		return err
	}
	if abs, err := store.Resolve(r.Path); err == nil {
		_ = os.Chtimes(abs, r.UpdatedAt, r.UpdatedAt)
	}
	return nil
}

// SyncOnce pulls once from every configured peer.
//
// A peer that fails to answer does not abort the round: the
// other peers still apply (a dead mesh member degrades to
// "offline in UI", never "management dead", CONCEPT req. 9).
// An error is returned only when every peer failed, so callers
// can tell "mesh is down" from "mesh is partial". In the
// single-peer case this is exactly the old behavior.
func SyncOnce(ctx context.Context, store *Store, ledger *Ledger, fetch Fetcher, peers []string) error {
	var errs []error
	for _, base := range peers {
		m, err := fetch.GetManifest(ctx, base)
		if err != nil {
			errs = append(errs, fmt.Errorf("sync: peer %s: %w", base, err))
			continue
		}
		if err := Apply(ctx, store, ledger, fetch, base, m); err != nil {
			return fmt.Errorf("sync: apply %s: %w", base, err)
		}
	}
	if len(errs) == len(peers) && len(errs) > 0 {
		return errs[0]
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
