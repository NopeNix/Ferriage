---
status: accepted
date: 2026-11-20
---

# 0008 — Audit log (Phase 3 item 5)

## Context
CONCEPT should-have: **audit log — who did what, when.** Two-zone
model (CONCEPT § Security): logged-in users are trusted operators;
the audit log *records* what they did, it does not constrain them.
It is Zone-2 data, not a Zone-1 gate — but it must still be
tamper-evident enough to be worth reading after the fact, and it
must never become a second source of truth for state.

## Decision
- **Storage: append-only JSONL on disk**, one line per event at
  `<stateDir>/.ferriage/audit.jsonl` (next to identity/ledger, so
  the sync walker — which never walks `.ferriage/` — can never
  replicate or delete it). State stays files (CONCEPT req. 6): the
  audit log is a *derived record*, not state; losing it loses
  history, never stacks. No database, no SQLite view (CONCEPT
  mentions SQLite for state metadata — audit history is not state
  metadata, and a second store for one append-only stream buys
  nothing).
- **Bounded**: the file is capped (default 10 000 lines,
  `FERRIAGE_AUDIT_MAX_LINES`). When an append would exceed the
  cap, the oldest lines are dropped by rewriting the file
  (temp file + rename, same pattern as sync writes). Retention is
  by count, not by time — predictable on a box that may be offline
  for months. A dropped tail is *not* silently zeroed: the rewrite
  keeps the newest lines verbatim, so gaps are visible as a time
  jump in the stream.
- **What is recorded** (closed action set, `internal/audit.Action`):
  `deploy`, `export`, `import`, `compose.save`, `exec`,
  `onboard`, `notify.test`. Each entry: `ts` (RFC 3339 UTC),
  `actor`, `action`, `target` (stack/host name, empty when the
  action has none), `status` (integer HTTP status of the
  operation), `detail` (short, human: e.g. `healthy=true` or the
  error text, capped at 200 chars). **No secrets in entries**:
  onboarding records the target address only — never the SSH key,
  never the request body. Exec records the command argv joined,
  capped — it is already trusted-operator data, but the *peer*
  must not become a key vault.
- **How it is recorded: middleware, not per-handler.** A request
  wrapper in `internal/audit` wraps the peer mux: after the
  handler returns it appends one line for every *mutating*
  request (`POST`/`PUT`/`PATCH`/`DELETE`) on the audited routes.
  Reads (`GET`) are never recorded — listing stacks every 30s
  would flood the log and bury the actions that matter. Peer-to-peer
  sync routes (`/sync/*`) and health/openapi are excluded: they
  are mesh machinery, not operator actions, and the sync puller
  runs with no actor. Recording is **best-effort and never fails
  the request**: an unwritable audit file logs to stderr and the
  operation proceeds (AGENTS.md §3: a dead host degrades, never
  "management dead" — the same applies to its log file).
- **Actor identity**: `X-Ferriage-Actor` header, set by the BFF
  from the verified OIDC session (`sub`). The peer is plaintext on
  the trusted net (ADR 0004), so the header is only trustworthy
  behind the BFF, exactly like the peer's other assumptions.
  Direct CLI calls without the header are recorded as
  `actor: "cli"` when `FERRIAGE_CLI=1` is not set — i.e. the CLI
  sends no actor header and the peer records the request as
  unauthenticated operator (`unknown`). Dev mode (BFF auth off)
  sends `dev`, mirroring the UI's dev session stub. The audit
  log is a record of *reported* identity, not an authentication
  boundary — it never gates anything.
- **Surfaces**: `GET /audit?limit=&actor=&action=&since=` (newest
  first, bounded to 500), CLI `ferriage audit [--limit N]
  [--actor U] [--action A]`, UI read-only `AuditLog` card next
  to `NotifyStatus`. All typed in `internal/api` + OpenAPI
  (drift-tested) + generated web types. There is **no write
  endpoint and no delete endpoint** — the log is append-only by
  API design, not by policy note.

## Consequences
- `internal/audit` is the only package that knows the log format;
  handlers stay unchanged except that mutating handlers run
  under the middleware (actor/status are captured from the
  request/response, not from handler internals).
- Audit entries are *per-peer*: each peer logs its own actions.
  Cross-peer correlation is future work (the mesh sync protocol
  carries state files, not audit history — replicating the log
  would let one peer forge another's history).
- Env knobs: `FERRIAGE_AUDIT_MAX_LINES` (default 10000),
  `FERRIAGE_AUDIT_DISABLE=1` (dev/testing escape hatch; the peer
  still serves `GET /audit` from whatever is on disk).
