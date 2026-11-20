---
status: accepted
date: 2026-11-20
---

# 0011 — Restic backup/restore polish: restic CLI-in-peer

## Context

Phase 3 item 8 (ROADMAP): incremental restic snapshots, retention, and
node-level disaster recovery — the "Backup / Restore / Export
(first-class)" section of CONCEPT. ADR 0002 already delivers the
one-file snapshot for moves (export/import), but a full snapshot per
backup is wasteful: unchanged volume blocks are re-stored every time,
and there is no cross-snapshot history or retention pruning.

Three packagings were on the table (session 018):

- **Sidecar container** — a separate restic container sharing the
  state volume. Clean image split, but a second container to run, a
  second wiring surface (repo path, password, scheduling), and it
  still needs the peer to know when to ask it for a backup.
- **CLI-in-peer (shell-out)** — the peer image ships the restic
  binary and the Go code execs it, exactly the way deploy already
  shells out to `docker compose` (session 009 decision: no new Go
  dependency for a mature CLI). Tests can point `FERRIAGE_RESTIC`
  at a fake binary.
- **Go library binding** — restic has no stable Go API; vendoring its
  internals would be building on the "rent, don't build" line's
  forbidden side.

## Decision

**restic runs as a CLI in the peer image, shelled out by
`internal/backup`** — the same posture as `docker compose` in
`internal/deploy`.

- **Binary**: the peer image (`deploy/Dockerfile`) installs the
  pinned official `restic` apk (Alpine's `restic` package). The
  runtime path is `FERRIAGE_RESTIC` (default `restic`).
- **Repository**: `.ferriage/restic-repo` under the state dir (never
  synced — `.ferriage/` is never walked by sync), created on first
  backup. The repo is the *backup engine's own storage*, not
  Ferriage state: losing it loses history, never stacks.
- **Password (ADR 0007 discipline)**: never crosses the API in
  either direction. `FERRIAGE_RESTIC_PASSWORD` (env, as the
  Shoutrrr URLs) or `FERRIAGE_RESTIC_PASSWORD_FILE` (a file the
  operator mounts, read for the password; Shoutrrr-style).
  `GET /backup` and status never echo the password or
  its source path beyond `password_set: bool`.
- **What gets backed up**:
  - Per-stack: the stack dir (Compose files) + captured volume
    contents — the same payload ADR 0002 snapshots, but kept
    *incremental and deduplicated in the repo*.
  - Node: the whole state dir minus `.ferriage/` (repos are
    per-node, identity/ledger/audit are per-node) — for disaster
    recovery, a node restores its stacks onto a fresh host.
- **Volumes are captured via the daemon copy APIs** (the
  `internal/snapshot` helper-container mechanism) into a temp tree,
  which is what restic actually backs up; restores re-populate
  fresh volumes the same way. No host filesystem access to
  `/var/lib/docker` is needed, and a remote `DOCKER_HOST` works.
- **Retention**: `restic forget --keep-last N --prune`
  (default `FERRIAGE_BACKUP_KEEP`, 7).
- **Restore verification**: a restore isn't done until the stack is
  healthy — restore calls the same health-gated deploy as
  `POST /stacks/{name}/deploy` (CONCEPT: health-gated, like a
  deploy).
- **Surfaces**: `POST /backup` (run now), `GET /backup` (repo +
  snapshot list, newest first), `POST /backup/restore`
  (+ health-gated deploy, optional), `POST /backup/forget` (apply
  retention), CLI `ferriage backup [run|list|restore|forget]`,
  MCP tools (`list_backups`, `run_backup`, `restore_backup`,
  `forget_backups`), `BackupStatus` UI card, Shoutrrr
  `backup.ok`/`backup.failed` events, audit actions
  `backup.run`/`backup.restore`/`backup.forget`.

## Consequences

- One new Go package (`internal/backup`), zero new Go deps, one new
  apk in the peer image. The Dockerfile's restic version is pinned
  by the base image's packaged version (Alpine), the same way
  `docker-cli`/`docker-cli-compose` already are.
- The existing ADR 0002 snapshot stays the *portable export*
  format (one file to move between peers); restic is the
  *incremental, retained* history on one node. They are
  complementary, not competing.
- A restic repo on a node that dies with its disk is gone — the
  repo itself is not mesh-synced (it's per-node, like audit).
  "Node backup" means "this node can be rebuilt from its own
  repo onto a fresh host", not "an off-site copy exists".
