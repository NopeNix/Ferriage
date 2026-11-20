# Ferriage — handover (current state)

> Rewritten at the end of every session. Keep it short (~40 lines).
> Frozen detail lives in `handover/`; learnings in
> `.openhands/memory/`. Read order: this file → memory index →
> `AGENTS.md`.

**Updated:** 2026-11-20, session 003
**Where the work is:** Phase 0 closed — stack confirmed as-is
(Go + Docker/Compose SDKs, React/shadcn UI, Iroh mesh; no ADR).
Product still concept-stage; no code yet. `CONCEPT.md` canonical.
**Stack:** Go peer, React/shadcn UI (separate container), Iroh mesh.
GitHub `NopeNix/Ferriage`, Docker Hub `nopenix/ferriage`.
**Still open:** dev IdP (Phase 2), state-sync mechanism (ADR before
Phase 1.3), first image tag (Phase 4 ship).
**Next session:** Phase 1.1 — install Go on home box, scaffold
`cmd/`, `internal/`, `go.mod`, health endpoint, `test.sh --fast`.
**Dev host:** home box (prototyping). **Prod:** VPS + LAN hosts —
never test mesh/agent changes against them without confirmation.
**Live instance:** none (nothing deployed yet).

## Session log

- [003 — phase 0 confirmation](handover/2026-11-20-003-phase0-confirmation.md) — stack confirmed, Docker Hub nopenix/ferriage; Phase 0 closed except deferred IdP + state-sync ADR; committed pending session 002
- [002 — roadmap](handover/2026-11-20-002-roadmap.md) — ROADMAP.md: phase 0 decisions → first milestone → UI shell → breadth → ship; minimum-viable record-keeping
- [001 — harness setup](handover/2026-11-20-001-harness-setup.md) — AGENTS.md, index+single-file memory/handover, docs skeleton, test.sh layers; open questions listed
