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
- **Volumes are captured from daemon-reported mountpoints.** The peer
  runs on the same host as the Docker daemon it manages (that's the
  deployment shape: peer container mounts the socket and
  `/var/lib/docker`), so `VolumeInspect` + a local walk is the whole
  capture path — no helper containers, no image pulls, no shelling
  out. Limitation: a peer pointed at a *remote* Docker host
  (`DOCKER_HOST` over TCP) can't read mountpoints; that needs a
  helper-container capture fallback, which is Phase 3 polish, not a
  format change.
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

- Phase 1.4 is one Go package (`internal/snapshot`) + two endpoints.
  Total new dependency surface: zero.
- Restic is **not** abandoned — it remains the Phase 3 choice for
  incremental/dedup backups. A future v2 format can add restic-repo
  export as an *additional* payload; v1 readers stay valid.
- gzip (stdlib) rather than zstd (busybox tar has no zstd flag, and
  Go's stdlib has no zstd): slightly larger snapshots, zero deps.
  Revisit if snapshots get big.
- Symlinks inside volume data are not followed (only regular files
  and dirs are captured) — same discipline as ADR 0001's sync walk.
