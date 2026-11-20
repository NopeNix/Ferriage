---
date: 2026-11-20
session: 017
slug: phase3-auto-update
topics: [phase3, auto-update, digest-poll, ADR-0009]
---

# Session 017 — Phase 3 (6/8): Auto-update

**Full routine `46 passed, 0 failed`.** ADR 0009 + `internal/update`
(per-node digest poll loop, `poll`/`auto` policy per stack,
stateful-default-poll heuristic, snapshot-before-update, Shoutrrr
on every event). Peer `GET /update` + `GET /update/{name}` +
`POST /update/{name}/check` + `PUT /update/{name}/policy` +
`POST /update/{name}/run`, CLI `ferriage update [check|policy|run]`,
UI `UpdateStatus` card next to `AuditLog` (+ baseline
`ui-2026-11-20-update-status.png`).

Live proof on the home box: peer against the real daemon —
`owui-tooling` reported `available` on startup (remote digest
differed from local), `demo` (stateful, alpine:3.20) reported
`available` under `poll`, `update policy demo --policy auto` +
`update run demo` snapshotted to
`snaps/demo/20261120T190029Z.tar.gz` and redeployed healthy;
Shoutrrr delivered `update.available`, `update.ok` via
`generic://` webhook to a local receiver; audit shows both
`peer/update.run` (machine) and `unknown/update.policy` +
`update.run` (operator). Fixed during proof: digest compare must
compare like-for-like (`RepoDigests[0]` vs manifest digest —
image ID is a different hash family and always "available");
stale peer on :18082 from session 016 was serving the old
binary (see memory).

Still open: Phase 3 items 7–8 (MCP, restic polish); first image
tag stays Phase-4 user-gated.

Live instance after this session: none running (dev stack
stopped, tmp state removed).

**Next session:** Phase 3 item 7 — MCP server (homelab is
AI-heavy). Design question to settle first: stdio vs HTTP
transport, and which peer endpoints become tools (likely
stacks/deploy/logs/exec/update).
