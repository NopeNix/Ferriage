---
date: 2026-11-20
session: 010
slug: phase1-6-compose-smoke
topics: [phase1.6, deploy, layer2, smoke]
---

# Session 010 — Phase 1.6: deploy files + layer-2 smoke

**Phase 1 is done (1.1–1.6).** Layer-2 smoke is green: full routine
`16 passed, 0 failed` (`--fast` 4/0 alone). The milestone flow it
proves — build peer image, two peers, deploy on B, **sync B→A**,
adopt by labels, export with volume, import on A, deploy on A,
**volume data survived the move**.

Files:

- `deploy/Dockerfile` — multi-stage. Runtime = `alpine:3.20` +
  `docker-cli` + `docker-cli-compose` + `ca-certificates` + `curl`.
  The peer deploys by shelling out to `docker compose up -d`

  (session 009) and reads the daemon through the bind-mounted
  socket; no Dockerfile change can break deploy without breaking
  the smoke. `CGO_ENABLED=0`, static binary, version via ldflags
  (`internal/version`, already plumbed since Phase 1.6's note).
- `deploy/docker-compose.yml` — one peer per node: `nopenix/ferriage`
  image, `/stacks` named volume, docker.sock bind, `curl`-based
  healthcheck, `FERRIAGE_PEER`/`FERRIAGE_SYNC_INTERVAL` knobs.
- `scripts/test.sh` layer 2 — replaces the `bad "not implemented yet"`
  placeholder with a real ~200-line flow above.

Smoke design (the parts that cost a few rounds each):

- Sync is **pull-only** (ADR 0001): A has `FERRIAGE_PEER=B`, B has
  none. So the stack dir is written on B, deployed on B, and the
  check is *B→A*. Writing on A and waiting for B is the classic
  wrong-direction trap — B never pulls.
- Peers run on `--network host` with fixed ports (18080/18081) and
  `FERRIAGE_ADDR`: port-publish + stop/start churn was the root of
  three separate flakes (host-gateway resolution, 127.0.0.1 vs
  gateway binding, port mapping after restart). Host networking
  removes the NAT variable entirely.
- Import refused with "stack already exists" until B was gone for
  the whole move: the synced copy on A is re-created within one
  500ms sync tick of any rm while B is live-selling it. Order:
  down → delete B's dir → stop both → rm A's copy → start only A
  → import → deploy → restart B → verify A's file **survives
  B's empty manifest** (delete-propagation rule: the restored file
  is newer than the ledger entry, so B's absence doesn't delete it).
- Peer state dirs are root-owned (containers run as root). The
  smoke cleans up via a throwaway root container; the operator's
  `rm -rf` silently leaves them otherwise. Cleanup verified by
  the leftovers check — none remain.

Lesson worth keeping: **during a live-500ms-sync move, kill the
whole side you're moving FROM, not just its containers.** The
sync heals path-level deletes by design (tombstones stop stale
peers). File-level surgery under a live peer's manifest always
loses.

Still open: Phase 2 (UI shell + Zone-1 OIDC; dev IdP per ROADMAP
Q2), first image tag (Phase 4). No merge/push/tag — user-gated
per AGENTS.md §6.

**Next session:** Phase 2 — first pixels + auth gate.
