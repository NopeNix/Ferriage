---
date: 2026-11-20
session: 003
slug: phase0-confirmation
topics: [phase0, stack, deployment]
---

# Session 003 — phase 0 confirmation

Operator confirmed the Phase 0 stack as specified in `CONCEPT.md`:
Go peer + Docker/Compose SDKs, React + shadcn/ui UI (separate
container), Iroh mesh. No deviations, so no ADR.

Docker Hub namespace confirmed: `nopenix/ferriage`
(`docs/deployment.md`, `ROADMAP.md`). First image tag still
deferred to first ship (Phase 4, user-gated).

Phase 0 is now closed except two items consciously deferred:
dev IdP (needed Phase 2) and state-sync mechanism (needed before
Phase 1.3, wire format gets an ADR). Next session: Phase 1.1
scaffolding (`cmd/`, `internal/`, `go.mod`, health endpoint).

Also committed the pending Session 002 work (roadmap, handover,
memory) in the same commit.
