---
date: 2026-11-21
session: 021
slug: phase4-release-path
topics: [phase4, release, version-stamping, docker, ci]
---

# Session 021 — Phase 4 release path (setup, no ship)

- Version stamping was broken at the Dockerfile level: ldflags
  referenced `${FERRIAGE_VERSION:-dev}` but no `ARG` declared it,
  so every image shipped as `dev`/`unknown` and the build warned
  "build-args not consumed". Fixed: `ARG` in both Dockerfiles +
  compose build args + `FERRIAGE_VERSION`/`FERRIAGE_COMMIT` ENV in
  the UI runtime stage. Peer reports both on GET /healthz (verified
  `v0.0.0-dryrun` + full SHA live); BFF now has its own
  unauthenticated GET /healthz for the container healthcheck
  (node:24-alpine has no curl/wget → probe via node fetch).
- `deploy/docker-compose.yml` grew the `ui` service
  (`nopenix/ferriage-ui:${FERRIAGE_TAG:-latest}`, OIDC env
  passthrough, `depends_on: peer healthy`, UI on :5173). Verified
  end-to-end: both containers healthy, version-stamped.
- Ship entrypoints (both need explicit human confirmation per
  AGENTS.md §6): `scripts/release.sh vX.Y.Z [--dry-run]` (clean
  tree + --fast + build + stamp-verify + typed confirmation +
  push/tag/release) and `.github/workflows/release.yml` (v* tag →
  Docker Hub + ghcr.io mirror + GitHub release; needs repo
  secrets DOCKERHUB_USERNAME/DOCKERHUB_TOKEN, not created yet).
- Docker Hub repo `nopenix/ferriage` does not exist yet (404);
  first push creates it. `docker login` not configured on dev box.
- Full routine still 67/0. Nothing pushed/tagged — Phase 4 ship
  itself is still operator-gated.
