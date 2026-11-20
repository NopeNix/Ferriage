---
date: 2026-11-20
session: 004
slug: phase1-scaffold
topics: [phase0-done, phase1.1, scaffold]
---

# Session 004 — Phase 0 confirmed done, Phase 1.1 scaffold

Phase 0 re-verified against HANDOVER/ROADMAP/CONCEPT and closed:
stack confirmed as-is, no ADR needed. Open questions carried
forward unchanged: dev IdP (Phase 2), state-sync mechanism (ADR
before Phase 1.3), first image tag (Phase 4).

Phase 1.1 complete on the home box (dev host): peer binary
skeleton + `/healthz`, `scripts/test.sh --fast` green (4/0),
live curl verified on :18080. No UI touched; no screenshot needed.

Next session: Phase 1.2 — state dir (stacks on disk), peer lists
stacks from Docker labels (adopt existing), prove with
`docker compose ps`. No Dockerfile/compose for Ferriage itself yet
(1.6); no merge/push/tag — user-gated per AGENTS.md §6.
