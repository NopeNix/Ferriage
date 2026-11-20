---
status: accepted
date: 2026-11-20
---

# 0009 — Auto-update (Phase 3 item 6)

## Context
CONCEPT should-have + the § "Auto-update (distributed)" section:
each node runs its own update loop (whale-mate, grown up) — digest
polling per stack, redeploy on change, Shoutrrr on every event.
Policy per stack: `poll` (report only) vs `auto` (redeploy).
Databases and stateful services default to `poll`, never `auto`.
Snapshot-before-update is mandatory for anything with state.

The peer is the right home for the loop: every node runs the same
binary and is a full peer (AGENTS.md §3) — a per-node loop needs no
control plane, and the UI stays a stateless reader.

## Decision
- **Policy per stack, stored as files**: `.ferriage/update.json`
  under the state dir (never synced — same rule as identity and
  the audit log: `.ferriage/` is never walked by sync). Shape:
  `{stacks: {<name>: {policy: "poll"|"auto", interval: "<duration>"}}}`.
  Defaults, when a stack has no entry:
  `FERRIAGE_UPDATE_POLICY` (default `poll`) and
  `FERRIAGE_UPDATE_INTERVAL` (default `24h`); a per-stack
  `interval` overrides the global. There is no per-stack
  `auto` grant by default — an operator opts a stack into `auto`
  via the API/CLI/UI. `FERRIAGE_UPDATE_DISABLE=1` turns the loop
  off entirely (the API/CLI surfaces keep working).
- **Database/stateful heuristic for the default**: a stack is
  "stateful" when its rendered Compose config declares a named
  volume (`volumes:` with a named source) — deterministic, no
  image-name guessing. Stateful stacks default to `poll` even
  when the global default is `auto`; they can be explicitly set
  to `auto`. Stateless stacks follow the global default. The
  heuristic outcome is surfaced (`stateful: true`) so the UI can
  explain why a stack didn't update itself.
- **The loop**: one background goroutine per peer, started from
  `cmd/peer`, ticking at the floor of the configured intervals
  (default tick `1h`; a per-stack interval smaller than the tick
  still fires on its own cadence via last-check tracking — the
  tick is a wakeup, not the schedule). Each tick, for every
  stack in the state dir whose interval has elapsed:
  1. **Digest check** per service image: local image inspect
     (`RepoDigests` / image ID) vs registry manifest
     (`DistributionInspect` via the daemon). A pinned digest
     reference (`image@sha256:…`) or a locally-built image with
     no registry digest is **unresolvable** for that service —
     recorded as `unresolvable`, never treated as "up to date".
     Services are checked independently; partial results are
     reported per service.
  2. **`poll` policy**: record the report (`available` /
     `up-to-date` / `unresolvable`), notify, done. No deploy, no
     snapshot.
  3. **`auto` policy**: when any service has a newer digest —
     **snapshot-first** (full ADR 0002 snapshot of the stack
     with its volumes, written to `.ferriage/snapshots/<stack>/
     <ts>.tar.gz`), then health-gated deploy, then notify with
     the outcome. The snapshot path is recorded in the report
     so an operator can roll back with `ferriage import`.
     Snapshot failure aborts the update (never deploy into a
     stateful stack without a restore point).
- **Shoutrrr on every event** (ADR 0007 extension): new closed
  events `update.available`, `update.ok`, `update.failed`,
  `update.snapshot_failed` — sent on every check that finds a
  newer digest, on every auto update outcome, and on snapshot
  failure. Best-effort like every other notification.
- **Surfaces**:
  - `GET /update` — all stacks: policy, stateful flag, last
    check, per-service digest report, last auto-update outcome.
  - `GET /update/{name}` — one stack's full report.
  - `POST /update/{name}/check` — force a check now (sync).
  - `PUT /update/{name}/policy` `{policy, interval?}` — set a
    stack's policy. Clears the "available" state so the next
    check re-evaluates.
  - `POST /update/{name}/run` — force an auto update now
    (snapshot + deploy) regardless of policy, for operators who
    want the full flow on demand. Audited.
  - CLI: `ferriage update [stack]`, `ferriage update check <stack>`,
    `ferriage update policy <stack> --policy poll|auto [--interval]`,
    `ferriage update run <stack>`.
  - UI: read-only `UpdateStatus` card next to `AuditLog`, plus
    per-stack policy display in `StackDetail`.
  All typed in `internal/api` + OpenAPI (drift-tested) + generated
  web types.
- **Audit**: `update.policy` and `update.run` are mutating operator
  actions and enter the audit log (middleware classifiers). The
  loop's own auto deploys are machine actions — recorded with
  actor `peer` (the loop is not an operator, and pretending it
  was one would forge history). `update.check` is a read-like
  probe and is *not* audited (like GETs, it changes no state
  except the transient report cache).
- **Image source of truth**: the *running* containers' image
  references (via `adopt` labels + inspect), not the Compose
  file text — the file may name a tag that moved, but the
  running digest is what an update would actually change. A
  stack with no running containers is reported `not-running`
  and skipped (nothing to update in place; deploy first).
- **No new daemon surface**: digest checks go through the
  existing Docker client (`DistributionInspect`); compose
  rendering reuses `docker compose config --format json` — no
  compose-go dependency (same rationale as deploy, session 009).

## Consequences
- `internal/update` is the only package that knows the policy
  store, the loop, and the digest compare; deploy/snapshot/notify
  stay untouched except for the new notify events.
- The peer gains a background loop and a snapshots directory.
  Snapshot retention is operator-managed for now (list them via
  the report; delete by hand) — retention policy is Phase 3.8
  (restic) territory.
- `FERRIAGE_UPDATE_*` env knobs: `INTERVAL`, `POLICY`,
  `DISABLE`. All default-safe: a fresh peer polls daily and
  never redeploys anything.
