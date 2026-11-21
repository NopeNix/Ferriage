package sync_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/NopeNix/Ferriage/internal/sync"
)

// meshPeer is one live HTTP peer in a multi-peer test mesh: a sync
// store served over httptest, its own ledger, and a fetch client.
// Mirrors how cmd/peer wires one instance.
type meshPeer struct {
	id     string
	root   string
	store  *sync.Store
	ledger *sync.Ledger
	srv    *httptest.Server
	url    string
}

func newMeshPeer(t *testing.T, id string) *meshPeer {
	t.Helper()
	root := t.TempDir()
	st := sync.New(root, id)
	led, err := sync.LoadLedger(root)
	if err != nil {
		t.Fatal(err)
	}
	st.SetOrigins(led.Origins())
	srv := httptest.NewServer(sync.NewHandler(st, id))
	t.Cleanup(srv.Close)
	return &meshPeer{id: id, root: root, store: st, ledger: led, srv: srv, url: srv.URL}
}

func (p *meshPeer) write(t *testing.T, path, data string) {
	t.Helper()
	full := filepath.Join(p.root, filepath.FromSlash(path))
	if err := os.MkdirAll(filepath.Dir(full), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(full, []byte(data), 0o644); err != nil {
		t.Fatal(err)
	}
	// Local writes are protected by the pull-baseline mtime
	// guard (see Apply); no ledger surgery here — the helper
	// stays faithful to production, where the sync package
	// has no "user wrote a file" hook.
	// Ensure strictly increasing mtimes across successive writes so
	// LWW ties inside one test never depend on clock granularity.
	if err := os.Chtimes(full, time.Now(), time.Now().Add(time.Second)); err != nil {
		t.Fatal(err)
	}
}

func (p *meshPeer) remove(t *testing.T, path string) {
	t.Helper()
	if err := os.Remove(filepath.Join(p.root, filepath.FromSlash(path))); err != nil {
		t.Fatal(err)
	}
}

func (p *meshPeer) read(t *testing.T, path string) (string, bool) {
	t.Helper()
	b, err := os.ReadFile(filepath.Join(p.root, filepath.FromSlash(path)))
	if err != nil {
		return "", false
	}
	return string(b), true
}

func (p *meshPeer) syncFrom(t *testing.T, from ...*meshPeer) {
	t.Helper()
	urls := make([]string, 0, len(from))
	for _, f := range from {
		urls = append(urls, f.url)
	}
	fetch := &sync.HTTPFetcher{Client: p.srv.Client()}
	if err := sync.SyncOnce(context.Background(), p.store, p.ledger, fetch, urls); err != nil {
		t.Fatalf("%s sync from %v: %v", p.id, urls, err)
	}
	p.store.SetOrigins(p.ledger.Origins())
}

func (p *meshPeer) mustHave(t *testing.T, path, want string) {
	t.Helper()
	got, ok := p.read(t, path)
	if !ok || got != want {
		t.Fatalf("%s: %s = %q (ok=%v), want %q", p.id, path, got, ok, want)
	}
}

func (p *meshPeer) mustNotHave(t *testing.T, path string) {
	t.Helper()
	if _, ok := p.read(t, path); ok {
		t.Fatalf("%s: %s should not exist", p.id, path)
	}
}

// converge runs the given peers' pull rounds until every peer holds
// identical content for every path in want. It is the multi-peer
// analogue of "eventually consistent": order of rounds does not
// matter, only that no further round changes anything.
func converge(t *testing.T, peers []*meshPeer, want map[string]string) {
	t.Helper()
	for round := 0; round < 6; round++ {
		for _, p := range peers {
			var others []*meshPeer
			for _, q := range peers {
				if q != p {
					others = append(others, q)
				}
			}
			p.syncFrom(t, others...)
		}
		done := true
		for _, p := range peers {
			for path, content := range want {
				if got, ok := p.read(t, path); !ok || got != content {
					done = false
				}
			}
		}
		if done {
			return
		}
	}
	for _, p := range peers {
		for path, content := range want {
			p.mustHave(t, path, content)
		}
	}
}

// TestThreePeerFanIn: three peers, each originates its own stack.
// C pulls from both A and B in one SyncOnce; all three converge.
// This is the >2 instance shape the 2-peer smoke test cannot see:
// a peer configured with TWO FERRIAGE_PEER entries.
//
//nolint:gocritic
func TestThreePeerFanIn(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "from-a")
	b.write(t, "db/compose.yaml", "from-b")
	c.write(t, "cache/compose.yaml", "from-c")

	// One SyncOnce call, two sources, on C.
	c.syncFrom(t, a, b)

	converge(t, []*meshPeer{a, b, c}, map[string]string{
		"web/compose.yaml":   "from-a",
		"db/compose.yaml":    "from-b",
		"cache/compose.yaml": "from-c",
	})
}

// TestThreePeerChainConverges: only pairwise links exist (A<->B,
// B<->C, no A<->C). A file written on A must still reach C, via B
// as a hop, once B has pulled it. Gossip through an intermediate
// is the mesh property single-link sync cannot provide.
func TestThreePeerChainConverges(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "a-only")
	// B pulls from A; C pulls only from B (never sees A directly).
	b.syncFrom(t, a)
	c.syncFrom(t, b)
	c.mustHave(t, "web/compose.yaml", "a-only")

	// Reverse direction: C writes, A receives only via B.
	c.write(t, "db/compose.yaml", "c-only")
	b.syncFrom(t, c)
	a.syncFrom(t, b)
	a.mustHave(t, "db/compose.yaml", "c-only")
}

// TestThreePeerPartitionHeals: C is unreachable while A and B
// keep syncing and A advances. When C returns, its next pull
// catches it up — including a file that was created AND edited
// while it was gone. A dead peer must not be "management dead":
// the remaining mesh keeps working.
func TestThreePeerPartitionHeals(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "v1")
	converge(t, []*meshPeer{a, b, c}, map[string]string{"web/compose.yaml": "v1"})

	// Partition C: close its server. A keeps writing.
	c.srv.Close()
	time.Sleep(20 * time.Millisecond)
	a.write(t, "web/compose.yaml", "v2-partitioned")
	a.write(t, "new/compose.yaml", "created-while-partitioned")

	// A <-> B still converge without C.
	b.syncFrom(t, a)
	a.syncFrom(t, b)
	b.mustHave(t, "web/compose.yaml", "v2-partitioned")
	b.mustHave(t, "new/compose.yaml", "created-while-partitioned")

	// C returns as a fresh instance on the same state dir
	// (its server address changes; state and ledger persist).
	srv2 := httptest.NewServer(sync.NewHandler(c.store, c.id))
	defer srv2.Close()
	c.url = srv2.URL
	c.srv = srv2
	c.syncFrom(t, a, b)
	c.mustHave(t, "web/compose.yaml", "v2-partitioned")
	c.mustHave(t, "new/compose.yaml", "created-while-partitioned")
}

// TestThreePeerDeadPeerDoesNotBlockOthers: a peer list whose
// FIRST entry is dead must not abort the pull from the live
// second entry. SyncOnce is per-peer tolerant: one dead mesh
// member degrades to "offline in UI", never blocks the rest.
func TestThreePeerDeadPeerDoesNotBlockOthers(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	live := newMeshPeer(t, "peer-live")
	dead := newMeshPeer(t, "peer-dead")

	live.write(t, "web/compose.yaml", "from-live")
	dead.srv.Close() // kill before anyone syncs from it

	// Dead first, live second — in a single SyncOnce.
	a.syncFrom(t, dead, live)
	a.mustHave(t, "web/compose.yaml", "from-live")

	// And the reverse order still works (no state corruption from
	// the failed first attempt).
	b := newMeshPeer(t, "peer-b")
	b.syncFrom(t, live, dead)
	b.mustHave(t, "web/compose.yaml", "from-live")
}

// TestThreePeerConcurrentEditConverges: A and B edit the same
// file concurrently while partitioned from each other. On heal,
// LWW (updated_at, peer_id, sha) picks one winner and ALL peers
// converge on it — no fork, no split-brain.
func TestThreePeerConcurrentEditConverges(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "base")
	converge(t, []*meshPeer{a, b, c}, map[string]string{"web/compose.yaml": "base"})

	// Partition: A and B both edit, neither sees the other.
	time.Sleep(20 * time.Millisecond)
	a.write(t, "web/compose.yaml", "edit-from-a")
	b.write(t, "web/compose.yaml", "edit-from-b")

	// Heal in both directions; every peer must agree on one value.
	converge(t, []*meshPeer{a, b, c}, map[string]string{})
	va, _ := a.read(t, "web/compose.yaml")
	vb, _ := b.read(t, "web/compose.yaml")
	vc, _ := c.read(t, "web/compose.yaml")
	if va != vb || vb != vc {
		t.Fatalf("fork after heal: a=%q b=%q c=%q", va, vb, vc)
	}
	if va != "edit-from-a" && va != "edit-from-b" {
		t.Fatalf("converged value %q is neither edit", va)
	}
	// B wrote later (strictly newer mtime), so B's edit must win.
	if va != "edit-from-b" {
		t.Fatalf("LWW winner = %q, want edit-from-b (newest write)", va)
	}
}

// TestThreePeerDeletePropagatesToAll: a delete on A removes the
// file on B and C, and a stale peer re-offering the old blob
// cannot resurrect it anywhere (tombstone honored mesh-wide).
func TestThreePeerDeletePropagatesToAll(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "to-be-deleted")
	converge(t, []*meshPeer{a, b, c}, map[string]string{"web/compose.yaml": "to-be-deleted"})

	time.Sleep(20 * time.Millisecond)
	a.remove(t, "web/compose.yaml")
	b.syncFrom(t, a)
	// C pulls from both configured sources: B has already
	// deleted it locally, so B's manifest completes the
	// all-sources-dropped vote and the delete fires.
	c.syncFrom(t, a, b)
	b.mustNotHave(t, "web/compose.yaml")
	c.mustNotHave(t, "web/compose.yaml")

	// The delete is stable: further full-mesh rounds must not
	// resurrect it on any peer. (A peer outside the mesh's
	// FERRIAGE_PEER lists cannot inject a stale copy —
	// production sync only pulls from configured sources; the
	// stale-source tombstone case is covered at unit level in
	// TestApplyPullsNewerAndDeletesTombstoned.)
	converge(t, []*meshPeer{a, b, c}, map[string]string{})
	b.mustNotHave(t, "web/compose.yaml")
	c.mustNotHave(t, "web/compose.yaml")
	a.mustNotHave(t, "web/compose.yaml")
}

// TestLocalDeleteIsNotHealedBack: the operator deletes a stack
// dir out-of-band on the ORIGIN peer (rm — there is no delete
// API; the state dir IS the API). The origin peer must record
// its own tombstone and refuse to re-pull the file from a mesh
// holder, and the delete must then propagate to the holders.
// Before the tombstone-scan fix, the originator resurrected
// its own delete from peer B forever and the stack never
// disappeared from the mesh.
func TestLocalDeleteIsNotHealedBack(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "operator-deleted")
	converge(t, []*meshPeer{a, b, c}, map[string]string{"web/compose.yaml": "operator-deleted"})

	// Operator rm on the origin peer, out-of-band.
	a.remove(t, "web/compose.yaml")

	// Full-mesh rounds: A must NOT heal it back from B or C,
	// and B and C must delete their copies (origin dropped it).
	converge(t, []*meshPeer{a, b, c}, map[string]string{})
	a.mustNotHave(t, "web/compose.yaml")
	b.mustNotHave(t, "web/compose.yaml")
	c.mustNotHave(t, "web/compose.yaml")
}

// TestBrandNewPeerJoinsThreePeerMesh: a fresh peer with local
// work, joining a mesh via its first pull, must never delete its
// own files (empty-ledger rule), and its files must flow out to
// the mesh on the peers' next pull.
func TestBrandNewPeerJoinsThreePeerMesh(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	b := newMeshPeer(t, "peer-b")
	c := newMeshPeer(t, "peer-c")

	a.write(t, "web/compose.yaml", "mesh-a")
	b.write(t, "db/compose.yaml", "mesh-b")

	// D joins with its own local stack, never synced before.
	d := newMeshPeer(t, "peer-d")
	d.write(t, "local/compose.yaml", "d-local-only")

	d.syncFrom(t, a, b)
	d.mustHave(t, "local/compose.yaml", "d-local-only") // not deleted
	d.mustHave(t, "web/compose.yaml", "mesh-a")
	d.mustHave(t, "db/compose.yaml", "mesh-b")

	// D's local work flows out to the existing mesh.
	converge(t, []*meshPeer{a, b, c, d}, map[string]string{
		"web/compose.yaml":   "mesh-a",
		"db/compose.yaml":    "mesh-b",
		"local/compose.yaml": "d-local-only",
	})
}

// TestSyncOnceDeadPeerReturnsErrorDirectly pins the raw SyncOnce
// contract used above: unreachable peers surface as errors, and
// SyncOnce does not silently swallow them. (Loop-level tolerance
// is covered by TestThreePeerDeadPeerDoesNotBlockOthers via the
// production path; this guards the primitive.)
func TestSyncOnceDeadPeerReturnsErrorDirectly(t *testing.T) {
	a := newMeshPeer(t, "peer-a")
	dead := newMeshPeer(t, "peer-dead")
	dead.srv.Close()

	fetch := &sync.HTTPFetcher{Client: &http.Client{Timeout: 2 * time.Second}}
	err := sync.SyncOnce(context.Background(), a.store, a.ledger, fetch, []string{dead.url})
	if err == nil {
		t.Fatal("SyncOnce from a dead peer must return an error")
	}
}
