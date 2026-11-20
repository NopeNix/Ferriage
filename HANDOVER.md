# Ferriage — handover (current state)

> Rewritten at the end of every session. Keep it short (~40 lines).
> Frozen detail lives in `handover/`; learnings in
> `.openhands/memory/`. Read order: this file → memory index →
> `AGENTS.md`.

**Updated:** 2026-11-20, session 010
**Where the work is:** Phase 0 closed. **Phase 1 DONE (1.1–1.6):**
peer+`/healthz`, state dir + adopt by labels, ADR 0001 manifest
file-sync + identity, ADR 0002 snapshot export/import (copy-API
volume capture), deploy with health gate, and now
**`deploy/Dockerfile` + `deploy/docker-compose.yml` + layer-2
smoke in `scripts/test.sh`** — full milestone flow green:
build peer image → two peers healthy → deploy on B → sync B→A →
adopt → export with volume → import on A → deploy on A →
**volume data survived the move**. `scripts/test.sh` full: 16/0.
`scripts/test.sh --fast`: 4/0. `CONCEPT.md` canonical.
**Stack:** Go peer, React/shadcn UI (separate container), Iroh mesh.
GitHub `NopeNix/Ferriage`, Docker Hub `nopenix/ferriage`.
**Still open:** Phase 2 UI shell (first pixels; dev IdP for OIDC);
first image tag (Phase 4 ship).
**Next session:** Phase 2 — minimal React/shadcn UI container
(stack list, peer status) + Zone-1 auth gate; pick dev IdP per
ROADMAP open question 2.
**Dev host:** home box (prototyping). **Prod:** VPS + LAN hosts —
never test mesh/agent changes against them without confirmation.
**Live instance:** none (nothing deployed yet; only smoke peers,
which are cleaned up).

## Session log

- [010 — phase 1.6 compose + smoke](handover/2026-11-20-010-phase1-6-compose-smoke.md) — deploy/Dockerfile + deploy/docker-compose.yml; layer-2 milestone smoke green (16/0); sync is pull-only, so checks are direction-aware
- [009 — phase 1.5 deploy](handover/2026-11-20-009-phase1-5-deploy.md) — POST /stacks/{name}/deploy + health gate; CLI not compose-go (one-line decision); live proof: healthy / no-healthcheck / 502-unhealthy / redeploy
- [008 — phase 1.4 finish](handover/2026-11-20-008-phase1-4-finish.md) — finished 007's uncommitted work; attach-stdin data loss fixed via daemon copy APIs; live round-trip + commit
- [007 — phase 1.4 snapshot](handover/2026-11-20-007-phase1-4-snapshot.md) — ADR 0002 snapshot export/import; two Docker-attach stdin gotchas
- [006 — phase 1.3 sync](handover/2026-11-20-006-phase1-3-sync.md) — ADR 0001 manifest file-sync + identity + tombstone ledger; two-peer live proof
- [005 — phase 1.2 adopt](handover/2026-11-20-005-phase1-2-adopt.md) — state dir + adopt by labels + GET /stacks proved live; docker deps pinned to Go 1.25
- [004 — phase 1.1 scaffold](handover/2026-11-20-004-phase1-scaffold.md) — Phase 0 re-verified and closed; peer binary + /healthz green, Go installed user-local
- [003 — phase 0 confirmation](handover/2026-11-20-003-phase0-confirmation.md) — stack confirmed, Docker Hub nopenix/ferriage; Phase 0 closed except deferred IdP + state-sync ADR; committed pending session 002
- [002 — roadmap](handover/2026-11-20-002-roadmap.md) — ROADMAP.md: phase 0 decisions → first milestone → UI shell → breadth → ship; minimum-viable record-keeping
- [001 — harness setup](handover/2026-11-20-001-harness-setup.md) — AGENTS.md, index+single-file memory/handover, docs skeleton, test.sh layers; open questions listed
