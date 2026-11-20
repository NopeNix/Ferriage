---
status: accepted
date: 2026-11-20
---

# 0001 — State sync: manifest-based file sync (not git-style)

## Context

Phase 1.3 requires replicating the stacks directory between peers
(ROADMAP open question #3). `AGENTS.md` §3 says wire/format changes need
a migration note and a decided ADR before implementation. Two candidates:

- **Git-style**: the state dir is a git repo (or bare clone), peers commit
  and fetch/push each other. Gives history, but also merge conflicts,
  commit identity, and a server-ish ref-update dance that fits poorly
  with NAT-safe, outbound-only, zero-touch mesh (every peer dials out,
  no inbound ports assigned by config).
- **File-sync**: peers exchange a content-addressed manifest of the
  stacks dir and fetch blobs by hash, applying them locally.
  No history, no merge conflicts — the newer write wins.

## Decision

**Manifest-based file sync.**

- **Manifest** (`GET /sync/manifest`): JSON list of entries
  `{path, sha256, size, mtime, updated_at, peer_id}` for every regular
  file under the state dir (stacks only; the peer's own identity/key/DB
  files are never synced). `path` is a relative `/`-separated path
  validated as a clean relative path — no `..`, no absolute paths, no
  symlinks followed, no cross-device moves.
- **Blob fetch** (`GET /sync/blob?path=…`): returns exact bytes for a
  validated path; the caller re-verifies `sha256` and refuses to apply
  a mismatch. Blob transfer is size-capped (default 64 MiB/file).
- **Conflict rule**: last-writer-wins per file, ordered by
  `(updated_at, peer_id)`; the content hash breaks remaining ties so
  all peers converge. `updated_at` is the last *local* write time of
  that file on the receiving writer, not a wall-clock guess at sync
  time. The manifest is rebuilt from the *actual* files on disk
  (hash, size, mtime read fresh) — there is no in-memory or ledger
  state that could go stale across restarts.
- **Deletes**: propagate as deletes when the remote manifest no longer
  lists a path the local tree has *and* the local entry must not be
  recreated. Recreation is suppressed by a durable per-peer provenance
  ledger: `path → (updated_at, peer_id, sha256)` recording what the
  last successful pull applied and the tombstones it honored (at
  `FERRIAGE_STATE_DIR/.ferriage/sync-ledger.json`, i.e. next to the
  identity, never synced). A remote delete is honored — never
  "healed" back from a second peer's stale manifest entry — while an
  explicit *local* rewrite (user edits the file, deleting the
  ledger's say-so) revives the path naturally. A brand-new peer with
  no ledger never applies deletes on its first sync from remote: it
  has no basis to know what a "delete" means there, so absence is
  treated as "never had it".
- **Identity**: each peer generates an ed25519 keypair on first start
  and persists it **outside** the synced tree (in a `.ferriage/`
  directory under the state dir root that sync never walks).
  `peer_id` = hex of the public key.
  It is the stable node identity for LWW + UI "offline" display and
  the seed for future signing. The keypair never leaves the host.
- **Transport split (kept, deliberately)**: this ADR fixes the *sync
  protocol* (manifest + blob + LWW + tombstone rule above). The
  *carrier* is out of scope here: Phase 1 uses plain HTTP to
  explicitly configured peers (`FERRIAGE_PEER` list), with
  `Peer-ID` headers and TLS termination left to the user's reverse
  proxy per CONCEPT req. 8. Swapping to Iroh/QUIC later changes the
  carrier, not the protocol, and needs no migration.
- **Future extensibility**: the on-disk layout stays plain files
  (compose files under `stacks/<name>/…`), so a future snapshot or
  git-style layer can be added on top without breaking consumers of
  the current wire format. Adding one later is a new ADR, not a
  migration of this one.

## Consequences

- Phase 1.3 is just: peer identity + manifest/blob endpoints + pull
  loop (`internal/sync`, `internal/identity`). No git binary, no
  merge machinery, no ref server.
- Volumes and images are **not** part of state sync; they move as
  snapshots (Phase 1.4/1.5), never as live mounts.
- LWW means concurrent edits to the same compose file on two peers
  lose one side without warning. Acceptable for the milestone
  (single operator, file is small); a lock/OT layer is a Phase 3+ ADR.
- Forward: the "peer dials out, no inbound config" rule means peers
  must be configured with the URLs/keys they pull from. Discovery
  (`iroh-gossip`) is Phase 3; until then `FERRIAGE_PEER` is explicit.
