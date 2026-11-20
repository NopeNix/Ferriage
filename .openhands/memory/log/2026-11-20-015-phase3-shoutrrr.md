---
date: 2026-11-20
session: 015
slug: phase3-shoutrrr
topics: [phase3, shoutrrr, notifications]
---

# Session 015 — Phase 3 (4/8): Shoutrrr notifications

**Full routine `40 passed, 0 failed`.** ADR 0007 (option A,
operator-chosen): `internal/notify` wraps
`github.com/containrrr/shoutrrr` v0.8.0; URLs env-only
(`FERRIAGE_NOTIFY_URLS`, comma-separated, wired in
`deploy/docker-compose.yml`); no endpoint accepts/returns
them. Events: `deploy.ok`/`deploy.failed`,
`onboard.ok`/`onboard.failed`, `notify.test`
(+ `peer.started` defined, not wired). Surfaces: `GET
/notify` (redacted), `POST /notify/test` (503 off / 502
delivery fail), CLI `ferriage notify status|test`, UI
read-only `NotifyStatus` card + Send-test button. Baseline
regenerated after operator sign-off.

Expensive-to-rediscover bits:
- Shoutrrr's `generic` service POSTs **HTTPS by default**;
  `?disabletls=yes` in the URL switches to plain HTTP
  (config derives `DisableTLS` from scheme http, and the
  query key overrides). Unit tests use a plain-HTTP
  `httptest` recorder with `?disabletls=yes` — a TLS test
  server fails on self-signed cert verify. Sender =
  `shoutrrr.CreateSender(urls...).Send(msg, &types.Params{})`
  which returns `[]error` (one per service); a nil receiver
  returns an error too.
- **Async send must not inherit the request ctx.** Hooks pass
  `r.Context()`, which is canceled when the handler returns —
  the send still delivered, but status recorded
  `last_error: "context canceled"`. `Notify` now ignores the
  caller's ctx and uses `context.Background()` + timeout.
- Shell/python edits in this repo: Go files are tab-indented
  (gofmt); patch anchors must use `\t`, and on the
  space-indented markdown/sh I should not guess — `cat -A`
  first. `str_replace` needs exact tabs.
- The peer on :8080 on this box is **Open WebUI**, not
  Ferriage — `go run ./cmd/peer` with defaults fails with
  "address already in use". Dev ports: 18080 peer, 19080 BFF.
- `scripts/test.sh` layer-3 baseline: regenerate by
  screenshotting the same throwaway peer+BFF (18082/19082,
  dist build) and `cp` over
  `tests/baselines/ui-2026-11-20-stack-list.png`; the
  script's own tmp dir is deleted by its cleanup, so run
  the peer/BFF manually for regen. Stale layer-3
  peer/BFF processes squat 18082/19082 — kill by PID.

Still open: Phase 3 items 5–8 (audit log, auto-update,
MCP, restic polish); first image tag stays Phase-4
user-gated.

Live instance after this session: none running (dev stack
stopped, tmp state removed).

**Next session:** Phase 3 item 5 — audit log (who did
what, when; Zone-2 records, doesn't constrain).
