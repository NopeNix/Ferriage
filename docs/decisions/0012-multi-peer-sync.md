---
status: accepted
date: 2026-11-20
---

# 0012 — Multi-peer sync: origin-gated deletes (amends 0001)

## Context

ADR 0001 defines the sync protocol (manifest + blob + LWW +
tombstone) but was only ever exercised with **two** peers: the
layer-2 smoke test (A pulls from B, B configured with no
`FERRIAGE_PEER`) and `TestTwoPeersSyncOverHTTP`. Real meshes
have ≥3 peers, each pulling from several sources. Extending the
tests to 3 peers (session 020) exposed four defects:

## Decision

- **Defects found (all fixed in `internal/sync`):**

  1. **Fan-in delete bug.** `Apply` deleted any local file
     absent from the current source's manifest. With C pulling
     from A and B, a file fetched from A was deleted by B's
     manifest (B never had it). Any peer configured with two
     sources ate files it had just pulled.
  2. **Tombstone lineage bug.** The tombstone recorded the
     manifest's `GeneratedAt` as its `UpdatedAt`, instead of
     the deleted entry's own lineage. The LWW comparison
     against a stale peer re-offering the old blob was run
     against the wall clock, not against what was deleted.
  3. **Dead peer blocks the list.** `SyncOnce` returned on the
     first unreachable source, so a dead entry earlier in
     `FERRIAGE_PEER` starved every healthy source after it.
  4. **Operator delete healed back.** There is no delete API —
     the operator `rm`s the stack dir. But the deleter's own
     ledger had no tombstone for the file, so on the same sync
     round it re-pulled the file from a mesh holder and
     re-served it forever. Deletes only worked in layer-2
     because the test stopped both peers during the move. In a
     live full mesh, `rm` did nothing.

- **Origin-gated deletes.** Each manifest entry now carries
  `origin_id`: the peer that last *wrote* the content, served
  from the ledger so copies never masquerade as originals.
  The ledger pins the origin per path; pulls record the
  entry's origin (falling back to the serving peer for
  pre-origin manifests); pulled files preserve the origin
  write-time as their mtime so identical content builds
  identical manifests everywhere (no pull-order mtime drift).
  A delete fires only when the **origin** peer drops the
  path. Absence from any other peer's manifest means "this
  source never had it" — which makes fan-in safe: C pulls
  web/ from A (origin A); B's manifest lacks web/ but C's
  origin for it is A, so nothing is deleted. Two earlier
  attempts failed, each caught by the next test layer:
  per-source unanimity votes deadlock once every peer holds
  a copy (each holder votes "present" forever), and
  last-server lineage flips when a holder's local mtime
  drift makes its copy "newer" than the originator's.
  Origin gating converges because the originator's absence
  is authoritative for its own content, and a newer copy
  held elsewhere re-pulls over the tombstone.
- **Local deletes become tombstones.** At the top of every
  `Apply`, any ledger-known synced path that has vanished
  from the state dir is converted to a tombstone before
  pulling anything. The peer then honors its own delete and
  refuses to re-pull the file from mesh holders; holders see
  the origin's absence and delete their copies. The refresh
  pass now records provenance for *all* local paths (origin
  self for local writes), so the originator's own locally
  created files are covered too. Only ledger-known paths
  qualify — never-synced local work is untouched, and a
  path still on disk (possibly edited) is never tombstoned.
- **Content-aware tombstones.** A tombstone keeps the deleted
  entry's lineage (`UpdatedAt`/`OriginID`/`SHA256`). The
  exact deleted bytes (same SHA) are honored-dead forever; a
  different, newer write revives the path — that is a
  genuine local rewrite, not a stale re-offer.
- **Per-peer error tolerance.** `SyncOnce` continues past a
  peer that fails to answer; it returns an error only when
  *every* peer failed. A dead mesh member degrades to
  "offline in UI", never "management dead" (CONCEPT req. 9).
  Single-peer behavior is unchanged.
- **Wire change (additive, backward compatible).**
  `SyncEntry.origin_id` (optional) is added to
  `/sync/manifest`, `/openapi.json`, and the generated web
  types. Pre-origin peers simply omit it; receivers treat an
  empty origin as "the serving peer is the origin", which is
  exactly the old single-source rule. No migration: old and
  new peers converge. Per AGENTS.md §3 this ADR *is* the
  required decision record for the wire addition.

## Consequences

- Peers can be configured with arbitrarily many
  `FERRIAGE_PEER` entries; fan-in, chain, partition-heal,
  concurrent-edit convergence, and delete propagation all
  hold with ≥3 instances.
- Deletes converge: the originator's out-of-band `rm`
  propagates to every peer whose origin points at it; a
  newer copy held elsewhere survives (edit beats delete by
  LWW, as the newest entry always wins).
- Coverage: 8 new Go integration tests
  (`internal/sync/mesh_test.go`) + Layer 2.5 in
  `scripts/test.sh` (3 peer containers, full mesh:
  fan-in, cross-peer edit, partition heal, delete
  propagation; dynamic free-port allocation + distinct
  identity checks so a stale dev peer can never silently
  hijack a mesh port).
