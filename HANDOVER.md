# Ferriage — handover (current state)

> Rewritten at the end of every session. Keep it short (~40 lines).
> Frozen detail lives in `handover/`; learnings in
> `.openhands/memory/`. Read order: this file → memory index →
> `AGENTS.md`.

**Updated:** 2026-11-20, session 012
**Where the work is:** Phases 0–2 done. Phase 1 (milestone, 1.1–1.6):
peer+`/healthz`, state dir + adopt by labels, ADR 0001 manifest
file-sync + identity, ADR 0002 snapshot export/import, health-gated
deploy, `deploy/Dockerfile`+compose + layer-2 smoke. **Phase 2 (UI
shell): `web/`** (Vite+React+Tailwind+shadcn, stack list + peer
status) **+ BFF Zone-1 OIDC gate** (ADR 0004, Dex dev IdP ADR 0003)
**+ layer-3 visual baseline**. **Phase 3 item 1 done: typed API +
CLI + stack detail** — `internal/api` contract behind
`/openapi.json`, `cmd/ferriage` CLI twin (stacks/peer/deploy/
export/import/version, `--json`), `StackDetail` view
(deploy/export/import + CSRF), layer-1.5 CLI e2e in `test.sh`.
Full routine `29/0` (layer1 6, CLI 10, layer2 12, layer3 1).
`CONCEPT.md` canonical.
**Stack:** Go peer, React/shadcn UI (separate container), Iroh mesh
(later). GitHub `NopeNix/Ferriage`, Docker Hub `nopenix/ferriage`.
**Still open:** Phase 3 items 2–8 (logs/stats/exec+Monaco, SSH
onboarding, Shoutrrr, audit, auto-update, MCP, restic polish);
first image tag (Phase 4 ship, user-gated).
**Next session:** Phase 3 item 2 — logs/stats/exec (stream,
stats, container console; Monaco editor with compose schema).
**Dev host:** home box (prototyping). **Prod:** VPS + LAN hosts —
never test mesh/agent changes against them without confirmation.
Dev ports on this box: peer 18080, BFF 19080, Dex 18081 (8080 is
Open WebUI). Kill the dev peer before `scripts/test.sh`.
**Live instance:** none (dev stack stopped after session 012).

## Session log

- [012 — phase 3 api+cli](handover/2026-11-20-012-phase3-api-cli.md) — internal/api typed contract + /openapi.json + drift test; cmd/ferriage CLI twin; StackDetail view; full routine 29/0
- [011 — phase 2 ui shell](handover/2026-11-20-011-phase2-ui-shell.md) — web/ + BFF OIDC gate (Dex dev IdP), layer-3 baseline, full routine 19/0
- [010 — phase 1.6 compose + smoke](handover/2026-11-20-010-phase1-6-compose-smoke.md) — deploy/Dockerfile + deploy/docker-compose.yml; layer-2 milestone smoke green (16/0); sync is pull-only, so checks are direction-aware
- [009 — phase 1.5 deploy](handover/2026-11-20-009-phase1-5-deploy.md) — POST /stacks/{name}/deploy + health gate; CLI not compose-go (one-line decision); live proof: healthy / no-healthcheck / 502-unhealthy / redeploy
- [008 — phase 1.4 finish](handover/2026-11-20-008-phase1-4-finish.md) — finished 007's uncommitted work; attach-stdin data loss fixed via daemon copy APIs; live round-trip + commit
- [007 — phase 1.4 snapshot](handover/2026-11-20-007-phase1-4-snapshot.md) — ADR 0002 snapshot export/import; two Docker-attach stdin gotchas
- [006 — phase 1.3 sync](handover/2026-11-20-006-phase2-ui-shell.md) — ADR 0001 manifest file-sync + identity + tombstone ledger; two-peer live proof
- [005 — phase 1.2 adopt](handover/2026-11-20-005-phase1-2-adopt.md) — state dir + adopt by labels + GET /stacks proved live; docker deps pinned to Go 1.25
- [004 — phase 1.1 scaffold](handover/2026-11-20-004-phase1-scaffold.md) — Phase 0 re-verified and closed; peer binary + /healthz green, Go installed user-local
- [003 — phase 0 confirmation](handover/2026-11-20-003-phase0-confirmation.md) — stack confirmed, Docker Hub nopenix/ferriage; Phase 0 closed except deferred IdP + state-sync ADR; committed pending session 002
- [002 — roadmap](handover/2026-11-20-002-roadmap.md) — ROADMAP.md: phase 0 decisions → first milestone → UI shell → breadth → ship; minimum-viable record-keeping
- [001 — harness setup](handover/2026-11-20-001-harness-setup.md) — AGENTS.md, index+single-file memory/handover, docs skeleton, test.sh layers; open questions listed
