---
date: 2026-11-20
session: 014
slug: phase3-ssh-onboarding
topics: [phase3, ssh, onboarding]
---

# Session 014 — Phase 3 (3/8): SSH onboarding

**Full routine `40 passed, 0 failed`.** ADR 0006 + `internal/onboard`
(native Go SSH, key-only, TO-FU/pinned host keys, Docker check +
get.docker.com install, compose render + `up -d`, healthz gate,
mesh link via `FERRIAGE_PEER`). Peer `POST /hosts/onboard` with
partial-steps 502, CLI `ferriage onboard`, UI `OnboardHost` card
+ regenerated stack-list baseline.

E2E proof on the dev box: onboarded `127.0.0.1` over the host
sshd as `user` → new peer healthy, own peer_id, known_hosts
stored; CLI twin green. Cleaned up all test artifacts.

Still open: Phase 3 items 4–8 (Shoutrrr, audit, auto-update, MCP,
restic polish); first image tag stays Phase-4 user-gated.

Live instance after this session: none running (dev stack stopped).

**Next session:** Phase 3 item 4 — Shoutrrr notifications
(built-in channel; events from deploy/deploy-failure/onboard).
