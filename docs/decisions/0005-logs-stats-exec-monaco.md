---
status: accepted
date: 2026-11-20
---

# 0005 — Logs/stats/exec + Monaco (Phase 3 item 2)

## Context

Phase 3 item 2 asks for log streaming, live stats, a container
console (exec/attach), and a proper code editor (Monaco with
compose schema autocomplete + validation). The wire-format and
Zone-1 rules apply: new peer endpoints need a decision before
implementation (`AGENTS.md` §7).

## Decision

**Polling JSON for logs/stats; a hand-rolled RFC 6455 WebSocket
for the console; Monaco vendored into the UI image.**

- **Logs** (`GET /stacks/{name}/services/{svc}/logs?tail=&since=`):
  bounded, non-follow JSON (`{container, lines[], truncated}`,
  each line `{ts, stream, message}`). The peer parses Docker's
  multiplexed frames itself (no daemon streaming needed for a
  tail), caps frames at 1MB, and never follows — the UI polls
  every 3s. `tail` is 1..10000 (default 200).
- **Stats** (`GET /stacks/{name}/stats`): one-shot
  (`ContainerStatsOneShot`) per container, CPU% via Docker's
  stats formula, memory + network from the same response. UI
  polls every 3s. No SSE/WebSocket for stats.
- **Exec** (`POST .../exec`, body `{cmd[], tty}`): one-shot
  create → attach → start → inspect, demultiplexed via
  stdcopy, output capped at 1MB. Manual `--` separator in the
  CLI; the UI sends the split argv array.
- **Console** (`WS .../console`): one exec session per
  connection, TTY `/bin/sh`. Wire: binary frames = terminal
  bytes; JSON text frames = `{"type":"resize","width","height"}`
  and `{"type":"close"}`. Implemented with zero new Go deps —
  the peer already hand-rolls wire formats (ADR 0001/0002) and
  the framing is ~70 lines. Upgrade 101 is hijacked from the
  stdlib mux.
- **Compose edit** (`GET|PUT /stacks/{name}/compose`): reads
  `<stateDir>/<name>/compose.yaml`; PUT replaces atomically via
  write-tmp + rename, capped at 4MB. The peer never parses the
  YAML — validation lives client-side in monaco-yaml against
  the compose schema. State stays a file (CONCEPT req. 6).
- **Frontend**: `ServiceTools` (logs/exec/console tabs per
  service), `StatsPanel`, `ComposeEditor` (Monaco +
  monaco-yaml, schema from compose-spec). `monaco-editor` is
  vendored into `dist/monaco` at build time
  (`scripts/copy-monaco.mjs`, `postbuild`) — the loader's
  default CDN pin is ignored (`loader.config({paths:{vs:
  "/monaco/vs"}})`), so the editor works offline on the LAN.
- **BFF**: `fetch()` cannot forward WebSocket upgrades, so
  `index.mjs` now handles `upgrade` with a raw TCP pipe to the
  peer, rewriting `/api/*` → peer path, with the same Zone-1
  session/CSRF proxy the HTTP path uses.
- **CLI twins**: `logs <stack> <svc> [--tail N]`, `stats <stack>`,
  `exec <stack> <svc> -- <cmd...>` (all `--json` capable).

## Consequences

- Logs/stats are transport artifacts, not state (ADR 0001/0002
  untouched): a dead host degrades to "offline in UI", never a
  sync problem.
- Hand-rolled WebSocket framing covers exactly the frames the
  console uses (text + binary, masked clients, JSON control).
  If a second WebSocket surface appears, revisit vendoring
  `gorilla/websocket` or `nhooyr`.
- The compose schema URL is a runtime fetch by monaco-yaml for
  validation only; with `enableSchemaRequest: false` the
  editor itself works offline, but schema diagnostics need
  one online fetch. Bundling the schema inline is the natural
  follow-up (also removes the GitHub dependency from the
  editor's happy path).
- Zone-1 dev mode: `getSession()` now treats `/auth/me` 404
  (no OIDC issuer) as an open dev session so browser writes
  work during prototyping; with OIDC on, the real session and
  CSRF token are used unchanged.
