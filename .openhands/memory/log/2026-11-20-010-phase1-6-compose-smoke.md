---
date: 2026-11-20
session: 010
slug: phase1-6-compose-smoke
topics: [phase1.6, deploy, layer2, smoke]
---

# Session 010 — Phase 1.6: deploy/Dockerfile+compose, layer-2 smoke

Phase 1 closed: `deploy/Dockerfile` (alpine + docker-cli +
docker-cli-compose; static CGO-off binary), `deploy/docker-compose.yml`
(one peer/node, /stacks volume, socket bind, curl healthcheck),
and layer-2 in `scripts/test.sh` green 16/0 — full milestone with
volume data surviving the move. Expensive lessons:

- **Sync direction matters in tests.** ADR 0001 sync is pull-only:
  only peers with `FERRIAGE_PEER` pull. A smoke that writes on the
  peer without `FERRIAGE_PEER` and waits for the other is the
  classic trap. Write on the peer that the other pulls from.
- **Host-network smoke peers.** `--network host` + `FERRIAGE_ADDR`
  removes three flake sources at once: host-gateway name
  resolution, 127.0.0.1-vs-gateway bind reachability, and
  port-mapping loss on stop/start.
- **File surgery under a live sync loses to sync, by design.**
  Deleting `a/<stack>` while B live-sells it re-syncs within one
  500ms tick; tombstones only stop *stale* peers. Move flow must
  kill the whole source side (containers + state dir) for the
  duration. (docs/design consequence, not a bug.)
- **Root-owned smoke state.** Peer containers run as root, so the
  operator's `rm -rf` fails silently on `.ferriage/`-containing
  dirs. Smoke cleanup needs a throwaway root container.
