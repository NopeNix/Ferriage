# Ferriage — handover (current state)

> Rewritten at the end of every session. Keep it short (~40 lines).
> Frozen detail lives in `handover/`; learnings in
> `.openhands/memory/`. Read order: this file → memory index →
> `AGENTS.md`.

**Updated:** 2026-11-21, session 020
**Where the work is:** Phases 0–2 done. Phase 1 (milestone, 1.1–1.6):
peer+`/healthz`, state dir + adopt by labels, ADR 0001 manifest
file-sync + identity, ADR 0002 snapshot export/import, health-gated
deploy, `deploy/Dockerfile`+compose + layer-2 smoke. **Phase 2 (UI
shell): `web/`** (Vite+React+Tailwind+shadcn, stack list + peer
status) **+ BFF Zone-1 OIDC gate** (ADR 0004, Dex dev IdP ADR 0003)
**+ layer-3 visual baseline**. **Phase 3 item 1 done: typed API +
CLI + stack detail** — `internal/api` contract behind
`/openapi.json`, `cmd/ferriage` CLI twin, `StackDetail` view.
**Phase 3 item 2 done: logs/stats/exec + Monaco** (ADR 0005) —
peer logs/stats/exec/compose handlers + hand-rolled WS console,
`ServiceTools`/`StatsPanel`/`ComposeEditor` in the UI, BFF raw
upgrade pipe, CLI logs/stats/exec twins. **Phase 3 item 3 done:
SSH onboarding** (ADR 0006) — native Go SSH, key-only, TO-FU
host keys, Docker check/install, compose render, healthz gate,
`POST /hosts/onboard`, `ferriage onboard`, `OnboardHost` UI
card. Full routine `40/0` (e2e: localhost sshd onboarded, new
peer healthy). **Phase 3 item 4 done: Shoutrrr notifications**
(ADR 0007, option A env-only) — `internal/notify`, redacted
`GET /notify` + `POST /notify/test`, CLI `ferriage notify`,
`NotifyStatus` UI card; live proof vs local webhook receiver,
baseline regenerated after operator sign-off. Full 40/0.
**Phase 3 item 5 done: audit log** (ADR 0008) —
append-only JSONL in `.ferriage/`, middleware-recorded
mutating actions (actor/action/target/status), BFF
`X-Ferriage-Actor` from the OIDC `sub`, read-only
`GET /audit`, CLI `ferriage audit`, `AuditLog` UI card;
live proof via bad-onboard POST (dev · onboard · 400 in the
browser). Full 46/0. **Phase 3 item 6 done: auto-update**
(ADR 0009) — `internal/update` per-node digest-poll loop,
`poll` vs `auto` policy in `.ferriage/update.json`, stateful
(named volumes) default `poll`, snapshot-before-update,
Shoutrrr `update.*` events, `GET /update` + check/policy/run,
CLI `ferriage update`, `UpdateStatus` UI card + baseline;
live proof vs real daemon (available reported, auto run
snapshotted + healthy, webhook delivered). Full 46/0.
**Phase 3 item 7 done: MCP server** (ADR 0010) —
`internal/mcp` hand-rolled MCP Streamable HTTP at `POST /mcp`
(JSON-RPC 2.0, stateless, `2025-03-26`, no SDK), 15 tools
dispatching in-process through the peer mux (same
handlers/validation/actor/audit as UI+CLI), BFF `/mcp`
proxy behind the OIDC gate, `ferriage mcp [tools|call]`
CLI twin, `audit.Record` for the inner action (envelope
never audited); live proof vs dev peer + BFF (initialize,
list, deploy, exec, audit-as-`mcp`). Full routine 53/0.
**Phase 3 item 8 done: restic backup/restore** (ADR 0011) —
restic CLI-in-peer (docker-compose shell-out precedent, not a
sidecar/binding), `internal/backup`: repo in
`.ferriage/restic-repo`, password env/file-only (only
`password_set` exposed), `GET /backup` + run (stack/node) +
restore (health-gated) + forget (retention 7), CLI `ferriage
backup`, 4 MCP tools (15→19), `BackupStatus` UI card, Shoutrrr
+ audit events; live proof rc 0.18.1: dedup (`data_blobs:0`),
down-v + restore → healthy with volume marker, MCP 19.
**Phase 3 is 8/8 complete.** Full routine 60/0.
**Post-Phase-3 (session 020): multi-peer E2E + ADR 0012** —
3-instance coverage at two layers (Go `mesh_test.go`, 8 tests;
shell Layer 2.5, 3 containers in full mesh) exposed and fixed
four sync defects: fan-in deletes (now origin-gated via new
additive wire `origin_id`), tombstone lineage (now content-
aware), dead-peer blocking the list (now per-peer tolerant),
and operator `rm` being healed back (now local-delete
tombstones). Full routine **67/0**. `CONCEPT.md` canonical.
**Stack:** Go peer, React/shadcn UI (separate container, Monaco
vendored offline), Iroh mesh (later). GitHub `NopeNix/Ferriage`,
Docker Hub `nopenix/ferriage`.
**Still open:** first image tag (Phase 4 ship, user-gated
per AGENTS.md §6).
**Next session:** Phase 4 ship — confirm the tag with the
operator, then tag → `gh release` → push `nopenix/ferriage`.
**Dev host:** home box (prototyping). **Prod:** VPS + LAN hosts —
never test mesh/agent changes against them without confirmation.
Dev ports on this box: peer 18080, BFF 19080, Dex 18081 (8080 is
Open WebUI). Kill the dev peer before `scripts/test.sh` —
layer 2/3 use fixed ports (poisoning, session 018); layer 2.5
allocates dynamic ports and is immune (session 020).
**Live instance:** none (dev stack stopped after session 020).

## Session log

- [020 — multi-peer e2e](handover/2026-11-21-020-multi-peer-e2e.md) — ADR 0012: origin-gated deletes (+origin_id wire), local-delete tombstones; 3-peer Go tests + container Layer 2.5; full routine 67/0
- [019 — phase 3 restic](handover/2026-11-20-019-phase3-restic.md) — ADR 0011: restic CLI-in-peer, incremental per-stack + node backups, retention, health-gated restore, CLI + MCP (19) + UI card; full routine 60/0
- [018 — phase 3 mcp](handover/2026-11-20-018-phase3-mcp.md) — ADR 0010: hand-rolled MCP Streamable HTTP at POST /mcp, 15 tools via in-process mux dispatch, BFF OIDC gate, inner-action audit; full routine 53/0
- [017 — phase 3 auto-update](handover/2026-11-20-017-phase3-auto-update.md) — ADR 0009: per-node digest-poll loop, poll/auto, stateful→poll, snapshot-first, Shoutrrr events; GET /update + CLI + UI card; full routine 46/0
- [016 — phase 3 audit](handover/2026-11-20-016-phase3-audit.md) — ADR 0008: audit JSONL in .ferriage/, middleware-recorded mutating actions, BFF actor header, read-only GET /audit + CLI + UI card; full routine 46/0
- [015 — phase 3 shoutrrr](handover/2026-11-20-015-phase3-shoutrrr.md) — ADR 0007: Shoutrrr env-only (FERRIAGE_NOTIFY_URLS), async best-effort, redacted GET /notify + test, CLI+UI twins; live proof vs local webhook; full routine 40/0
- [014 — phase 3 ssh onboarding](handover/2026-11-20-014-phase3-ssh-onboarding.md) — ADR 0006: native x/crypto/ssh, key-only, TO-FU host keys, Docker check/install, POST /hosts/onboard, ferriage onboard, OnboardHost UI; e2e localhost sshd onboarded; full routine 40/0
- [013 — phase 3 logs/stats/exec](handover/2026-11-20-013-phase3-logs-stats-exec.md) — ADR 0005: logs/stats/exec + WS console (hand-rolled RFC6455) + Monaco vendored offline + BFF raw upgrade pipe; full routine 40/0
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
