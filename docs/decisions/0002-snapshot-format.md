---
status: accepted
date: 2026-11-20
---

# 0002 — Snapshot format: single gzip+tar file, Go-written (not a restic repo)

## Context

Phase 1.4 (ROADMAP): snapshot a stack **plus its volumes** into a
portable file; restore it round-trip. CONCEPT says backup should be
"restic-style" and "rent, don't build". Two candidates:

- **Restic repo**: real incremental/dedup engine, but a *repository*
  is a directory of pack files + a key. It needs the restic binary (or
  a Go library with its own storage abstraction) on every peer, a
  password story, and multi-file handling for the "export = one file
  the operator can move" flow. That's the backup *engine* — right for
  Phase 3's incremental backups, heavy for the first milestone.
- **Single tarball**: one `.tar.gz` file containing the stack files,
  the volume contents, and a small JSON manifest. Written and read by
  Go stdlib (`archive/tar` + `compress/gzip`) — no binary dependency,
  no password, streams over HTTP in both directions.

## Decision

**Single gzip+tar snapshot file, format version 1.**

Layout (all paths relative to the archive root, `/`-separated):

```
manifest.json          # format metadata (first entry)
stack/                 # the stack dir contents (compose files, env, …)
volumes/<name>/        # content of each named volume, by volume name
```

`manifest.json`:

```json
{
  "format_version": 1,
  "stack": "myapp",
  "created_at": "2026-11-20T13:00:00Z",
  "created_by": "<peer_id>",
  "files": ["stack/compose.yaml", "volumes/app-data/db/x"],
  "volumes": [{"name": "app-data"}]
}
```

- `files` is the complete entry list (excluding `manifest.json`
  itself); restore verifies every entry's path is clean and safely
  contained (same `ValidatePath` discipline as ADR 0001) — a hostile
  archive can never escape the destination dirs.
- **Volumes are captured via the daemon's copy APIs**
  (`CopyFromContainer` / `CopyToContainer`) through a throwaway
  `alpine` helper container with the volume bound at `/vol` — the
  daemon tars the mountpoint itself, so no mountpoint paths leak into
  the peer and no host filesystem access to `/var/lib/docker` is
  needed, and a peer pointed at a *remote* Docker host
  (`DOCKER_HOST` over TCP) works too.
  **Volume data never travels the helper's attach stream:** the
  first cut piped the tar through attach stdin (`tar -xvf -`) and
  was wrong on this stack — attach stdin of a not-yet-started
  container silently delivers zero bytes (verified live), so
  restores failed "short read" and even plain `cat` received
  nothing. The helper survives only as a `sleep` anchor whose
  volume is the copy target. Copy entries arrive rooted at the
  copied dir's base name (`vol/...`); they are rebased to
  volume-relative paths.
- **Restore creates volumes fresh**: volumes are recreated as
  `<stack>_<name>` (compose's default project prefix), and their
  contents are written into the new mountpoints. If the destination
  stack dir or volume already exists, restore **refuses** — no silent
  overwrite of live data. `--force`-style overwrite is a later,
  explicit decision.
- Export is served over the peer API (`POST /stacks/{name}/export`,
  streaming) so the same flow works locally or peer-to-peer later;
  import is `POST /stacks/{name}/import` (body = snapshot file).

## Consequences

- Phase 1.4 ships as one Go package (`internal/snapshot`) + two
  endpoints (`POST /stacks/{name}/export|import`), zero new
  dependency surface. Round-trip proved live on the dev host
  (export → `down -v` → import → `up` → volume data intact).
- Restic stays the Phase 3 candidate for incremental/dedup backups.
  A future format version can add restic as an *additional* payload.
- gzip (stdlib), not zstd (Go's stdlib has no zstd): slightly
  larger snapshots, zero deps. Revisit if snapshots get big.
- Symlinks inside volume data are dropped on capture (daemon tar
  resolves them); restore materializes regular files — same
  discipline as ADR 0001's sync walk.
