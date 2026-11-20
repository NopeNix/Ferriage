---
date: 2026-11-20
session: 015
slug: phase3-shoutrrr
topics: [phase3, shoutrrr, notifications]
---

# Session 015 — Phase 3 (4/8): Shoutrrr notifications

**Full routine `40 passed, 0 failed`.** ADR 0007 + `internal/notify`
(Shoutrrr v0.8.0, env-only `FERRIAGE_NOTIFY_URLS`, async
best-effort, peer-id-prefixed). Peer `GET /notify` (redacted) +
`POST /notify/test`, CLI `ferriage notify status|test`, UI
`NotifyStatus` card (operator sign-off, baseline regenerated).
Events: deploy ok/failed, onboard ok/failed, test.

Live proof on the home box: peer with a local webhook receiver;
`notify test` + `deploy demo` both delivered
(`Ferriage deploy.ok demo: peer ac43… healthy`); disabled and
enabled UI states screenshotted. Fixed during proof: async send
must not inherit the request ctx (`last_error: "context
canceled"` wart).

Still open: Phase 3 items 5–8 (audit log, auto-update, MCP,
restic polish); first image tag stays Phase-4 user-gated.

Live instance after this session: none running (dev stack
stopped, tmp state removed).

**Next session:** Phase 3 item 5 — audit log (who did what,
when; Zone-2 records, doesn't constrain).
