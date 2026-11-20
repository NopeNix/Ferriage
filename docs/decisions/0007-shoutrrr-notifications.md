---
status: accepted
date: 2026-11-20
---

# 0007 — Shoutrrr notifications (Phase 3 item 4)

## Context
CONCEPT should-have: **Shoutrrr** notifications (the built-in
channel) for lifecycle events — deploy outcomes and host
onboarding, with auto-update events landing in Phase 3 item 6.
Shoutrrr URLs are secrets: Slack webhook URLs, Telegram bot
tokens, SMTP credentials.

## Decision
- **Option A (env-only, operator-chosen)**: Shoutrrr URLs come
  only from `FERRIAGE_NOTIFY_URLS` (comma-separated) in the peer
  environment, wired through `deploy/docker-compose.yml`. The
  peer has **no endpoint that accepts or returns the URLs** —
  the secret never crosses the API in either direction, so there
  is no secret-writing API surface to harden (Zone 1) and no
  secret file to protect on disk. Changing a URL = compose env
  edit + peer restart.
- **Events** (closed set, `internal/notify.Event`):
  `deploy.ok`, `deploy.failed`, `onboard.ok`, `onboard.failed`,
  `notify.test`, `peer.started` (planned; not yet wired).
  Messages are prefixed with the peer id (`Ferriage
  deploy.ok demo: healthy (peer abc…)`) so a mesh operator can
  tell which peer fired.
- **Best-effort, always**: sends run asynchronously with a 10s
  bound; a dead webhook is logged and recorded, never returned
  to the deploy/onboard caller, never blocks the response.
- **Surfaces**: `GET /notify` (redacted status: enabled, channel
  count, last event/time/error — never URLs or bodies),
  `POST /notify/test` (synchronous send; 503 when unconfigured,
  502 on delivery failure), CLI `ferriage notify status|test`,
  UI read-only `NotifyStatus` card next to `OnboardHost` with a
  Send-test button. Both typed in `internal/api` + OpenAPI
  (drift-tested) + generated web types.
- **Library**: `github.com/containrrr/shoutrrr` v0.8.0
  (CONCEPT's named component). Invalid URLs are counted and
  skipped at startup — one typo doesn't silence other channels,
  and the peer never fails to start over notifications.

## Consequences
- `internal/notify` is the only package that knows Shoutrrr;
  deploy/onboard handlers get an optional `*notify.Notifier`
  (nil = disabled) and stay unconditional.
- The peer image gains Shoutrrr's service tree (Slack, Discord,
  Telegram, SMTP, webhooks, …) — acceptable for a static Go
  binary; no new runtime dependency.
- Operator ergonomics: no UI to paste a webhook into. That is
  the deliberate trade — the secret stays in env/compose where
  operators already keep it (AGENTS.md §4: API keys over
  passwords, per-host env files).
