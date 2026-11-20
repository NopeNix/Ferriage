# Ferriage — handover (current state)

> Rewritten at the end of every session. Keep it short (~40 lines).
> Frozen detail lives in `handover/`; learnings in
> `.openhands/memory/`. Read order: this file → memory index →
> `AGENTS.md`.

**Updated:** 2026-11-20, session 009
**Where the work is:** Phase 0 closed. Phase 1.1–1.5 done: peer+
`/healthz`, state dir + adopt by labels, ADR 0001 manifest file-sync +
identity, ADR 0002 snapshot export/import (copy-API volume capture),
and **`POST /stacks/{name}/deploy` (compose CLI + Engine-API health
gate; 502 with per-service state on timeout; idempotent redeploy)**.
`scripts/test.sh --fast`: 4/0. `CONCEPT.md` canonical.
**Stack:** Go peer, React/shadcn UI (separate container), Iroh mesh.
GitHub `NopeNix/Ferriage`, Docker Hub `nopenix/ferriage`.
**Still open:** layer-2 smoke + `deploy/docker-compose.yml` (1.6);
dev IdP (Phase 2); first image tag (Phase 4 ship).
**Next session:** Phase 1.6 — `deploy/docker-compose.yml` + layer-2
compose smoke (two peers, sync, adopt, move-with-volume,
health-gated deploy) in `scripts/test.sh`.
**Dev host:** home box (prototyping). **Prod:** VPS + LAN hosts —
never test mesh/agent changes against them without confirmation.
**Live instance:** none (nothing deployed yet).

## Session log

- [009 — phase 1.5 deploy](handover/2026-11-20-009-phase1-5-deploy.md) — POST /stacks/{name}/deploy + health gate; CLI not compose-go (one-line decision); live proof: healthy / no-healthcheck / 502-unhealthy / redeploy
- [008 — phase 1.4 finish](handover/2026-11-20-008-phase1-4-finish.md) — finished 007's uncommitted work; attach-stdin data loss fixed via daemon copy APIs; live round-trip + commit
- [007 — phase 1.4 snapshot](handover/2026-11-20-007-phase1-4-snapshot.md) — ADR 0002 snapshot export/import; two Docker-attach stdin gotchas
- [006 — phase 1.3 sync](handover/2026-11-20-006-phase1-3-sync.md) — ADR 0001 manifest file-sync + identity + tombstone ledger; two-peer live proof
- [005 — phase 1.2 adopt](handover/2026-11-20-005-phase1-2-adopt.md) — state dir + adopt by labels + GET /stacks proved live; docker deps pinned to Go 1.25
- [004 — phase 1.1 scaffold](handover/2026-11-20-004-phase1-scaffold.md) — Phase 0 re-verified and closed; peer binary + /healthz green, Go installed user-local
- [003 — phase 0 confirmation](handover/2026-11-20-003-phase0-confirmation.md) — stack confirmed, Docker Hub nopenix/ferriage; Phase 0 closed except deferred IdP + state-sync ADR; committed pending session 002
- [002 — roadmap](handover/2026-11-20-002-roadmap.md) — ROADMAP.md: phase 0 decisions → first milestone → UI shell → breadth → ship; minimum-viable record-keeping
- [001 — harness setup](handover/2026-11-20-001-harness-setup.md) — AGENTS.md, index+single-file memory/handover, docs skeleton, test.sh layers; open questions listed
