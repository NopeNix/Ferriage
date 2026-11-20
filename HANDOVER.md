# Ferriage — handover (current state)

> Rewritten at the end of every session. Keep it short (~40 lines).
> Frozen detail lives in `handover/`; learnings in
> `.openhands/memory/`. Read order: this file → memory index →
> `AGENTS.md`.

**Updated:** 2026-11-20, session 006
**Where the work is:** Phase 0 closed. Phase 1.1 (peer skeleton +
`/healthz`), 1.2 (state dir, adopt by labels, `GET /stacks`), and
**1.3 (ADR 0001 manifest file-sync + ed25519 identity + tombstone
ledger; `GET /sync/manifest|/blob`, `/peer`; two-peer live proof:
write/edit/delete all propagated both ways)** done. State dir
`FERRIAGE_STATE_DIR` (default `./stacks`), peers via `FERRIAGE_PEER`
list, `FERRIAGE_SYNC_INTERVAL`. `scripts/test.sh --fast`: 4/0.
`CONCEPT.md` canonical.
**Stack:** Go peer, React/shadcn UI (separate container), Iroh mesh.
GitHub `NopeNix/Ferriage`, Docker Hub `nopenix/ferriage`.
**Still open:** export with volumes (1.4); dev IdP (Phase 2);
first image tag (Phase 4 ship).
**Next session:** Phase 1.4 — snapshot stack + volumes → portable
file (restic), round-trip restore test on the same host.
**Dev host:** home box (prototyping). **Prod:** VPS + LAN hosts —
never test mesh/agent changes against them without confirmation.
**Live instance:** none (nothing deployed yet).

## Session log

- [006 — phase 1.3 sync](handover/2026-11-20-006-phase1-3-sync.md) — ADR 0001 manifest file-sync + identity + tombstone ledger; two-peer live proof
- [005 — phase 1.2 adopt](handover/2026-11-20-005-phase1-2-adopt.md) — state dir + adopt by labels + GET /stacks proved live; docker deps pinned to Go 1.25
- [004 — phase 1.1 scaffold](handover/2026-11-20-004-phase1-scaffold.md) — Phase 0 re-verified and closed; peer binary + /healthz green, Go installed user-local
- [003 — phase 0 confirmation](handover/2026-11-20-003-phase0-confirmation.md) — stack confirmed, Docker Hub nopenix/ferriage; Phase 0 closed except deferred IdP + state-sync ADR; committed pending session 002
- [002 — roadmap](handover/2026-11-20-002-roadmap.md) — ROADMAP.md: phase 0 decisions → first milestone → UI shell → breadth → ship; minimum-viable record-keeping
- [001 — harness setup](handover/2026-11-20-001-harness-setup.md) — AGENTS.md, index+single-file memory/handover, docs skeleton, test.sh layers; open questions listed
