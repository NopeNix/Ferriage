---
date: 2026-11-20
session: 013
slug: phase3-logs-stats-exec
topics: [phase3, logs, stats, exec, monaco, websocket, bff]
---

# Session 013 — Phase 3 (2/8): logs/stats/exec + Monaco

**Full routine `40 passed, 0 failed`** (layer 1: 6, layer 1.5
CLI: 20, layer 2: 12, layer 3: 1). What shipped:

- ADR 0005: polling JSON for logs/stats, hand-rolled RFC 6455
  WebSocket console (zero new Go deps), Monaco vendored into
  the UI image (offline, no CDN), raw TCP upgrade proxy in
  the BFF for WebSocket.
- Peer: `internal/logs` (bounded non-follow tails, own frame
  parser), `internal/stats` (one-shot CPU/mem/net), `internal/exec`
  (one-shot exec, stdcopy demux, 1MB cap), `internal/compose`
  (GET|PUT compose.yaml, atomic rename, 4MB cap),
  `internal/console` (WS console, TTY `/bin/sh`, JSON
  resize/close control). Typed API + OpenAPI + drift test
  extended (20 schemas).
- UI: `ServiceTools` (logs 3s poll, exec, console with
  resize), `StatsPanel` (3s poll), `ComposeEditor` (Monaco +
  monaco-yaml, schema-backed). Dev-mode session fix in
  `client.ts` (404 on /auth/me = open dev session).
- BFF: `upgrade` handling — raw TCP pipe to peer for `/api/*`
  WebSocket (fetch() 502s on upgrades; verified 101 after).
- CLI: `logs <stack> <svc> [--tail N]`, `stats <stack>`,
  `exec <stack> <svc> -- <cmd...>`.

Real-browser proof (Playwright Chromium): stack list, live log
tail, exec `ls /` exit 0, interactive console (`echo
hello-console` round-trip), stats panel, Monaco compose render —
all against the dev peer+BFF. New baseline committed:
`tests/baselines/ui-2026-11-20-stack-detail-tools.png`.

Gotchas:

- `errdefs.IsNotFound` (docker) is staticcheck-deprecated;
  use `github.com/containerd/errdefs` (already indirect in
  go.mod).
- `@monaco-editor/loader` pins its own monaco (0.55.1) via CDN;
  override with `loader.config({paths:{vs:"/monaco/vs"}})` +
  `postbuild` vendor copy — version skew avoided, offline kept.
- `file_editor`/`sed -n` matching is literal: files use tabs,
  so `str_replace` with space-indented anchors fails silently.
- The BFF returned 502 on WebSocket upgrades before the raw
  pipe; fetch-based proxying can never do upgrades.

Still open: Phase 3 items 3–8 (SSH onboarding, Shoutrrr,
audit log, auto-update, MCP, restic polish). First image tag
stays Phase-4 user-gated.

Live instance after this session: none running (dev stack
stopped; run with `FERRIAGE_ADDR=:18080 … go run ./cmd/peer`,
`PORT=19080 … node web/bff/index.mjs`).

**Next session:** Phase 3 item 3 — SSH onboarding (zero-touch:
address + key → Docker check → one Ferriage container).
