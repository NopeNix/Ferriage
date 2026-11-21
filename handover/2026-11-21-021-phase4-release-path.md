---
date: 2026-11-21
session: 021
slug: phase4-release-path
topics: [phase4, release, version-stamping, docker, ci]
---

# Session 021 — Phase 4 release path (setup, no ship)

Follow-up to session 020: with Phases 0–3 all green, prepared the
Phase 4 ship path without executing it (tag/release/push remain
operator-gated per AGENTS.md §6).

## What changed

- **Version stamping fixed.** `deploy/Dockerfile` referenced
  `${FERRIAGE_VERSION:-dev}` in ldflags but declared no `ARG`, so
  images always shipped as `dev`/`unknown`. Added
  `ARG FERRIAGE_VERSION`/`FERRIAGE_COMMIT` in both Dockerfiles
  (UI promotes them to ENV in the runtime stage), compose build
  args, and a BFF `GET /healthz` (unauthenticated, build identity).
- **Compose now ships both services.** `deploy/docker-compose.yml`
  added `ui` (`nopenix/ferriage-ui`, OIDC passthrough,
  health-gated on peer). Naming mirrors the peer image.
- **Two ship entrypoints, neither run:** `scripts/release.sh`
  (manual, `--dry-run` for build+verify only, typed confirmation
  before the irreversible half) and `.github/workflows/release.yml`
  (v* tag → Docker Hub + ghcr.io + GitHub release).
- `docs/deployment.md` documents images, stamping, both ship
  routes, and the required CI secrets.

## Verification (home box)

- `scripts/test.sh --fast` 6/0; full routine **67/0**.
- Peer image built with `v0.0.0-dryrun` stamps → `/healthz`
  returns `{"version":"v0.0.0-dryrun","commit":"d20c468…"}`.
- UI image → BFF `/healthz` returns the same stamp; full compose
  stack up: peer + UI both `(healthy)`, SPA served on :15173.

## Still open (operator-gated)

- First ship: confirm tag (suggested `v0.1.0`), then
  `scripts/release.sh v0.1.0 --dry-run`, confirm, ship — or push a
  `v*` tag and let CI do it (needs `DOCKERHUB_USERNAME` +
  `DOCKERHUB_TOKEN` repo secrets; `docker login` not configured
  locally; Docker Hub repo `nopenix/ferriage` still 404s).
- Nothing pushed, tagged, or released this session.
