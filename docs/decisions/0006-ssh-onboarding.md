---
status: accepted
date: 2026-11-20
---

# 0006 — SSH onboarding (Phase 3 item 3)

## Context
CONCEPT hard requirements 11–12: Ferriage connects to hosts over
SSH; on first contact it checks Docker + the Compose plugin and
installs them if missing, then starts one Ferriage container
(persistent volume). Onboarding a new host = address + SSH key.
`docs/deployment.md` already specified the flow; this ADR pins
the implementation choices.

## Decision
- **Transport**: native Go SSH (`golang.org/x/crypto/ssh`) — no
  `openssh-client` dependency in the peer image, same
  "pure Go, static binary" rule as the Docker SDK usage.
- **Auth**: key-only by default (address + key path, per
  CONCEPT). Password auth is never accepted by the peer API.
  The key is loaded client-side (UI/CLI) and passed in the
  request body; the peer keeps it in memory for one connection.
- **Host keys**: operator pins via `FERRIAGE_SSH_KNOWN_HOSTS`
  (file path, openssh format). With no pin file the peer
  accepts-and-verifies with a TO-FU store under the state dir
  (`.ferriage/known_hosts`), logging the key fingerprint it
  accepted. Pinning is recommended for prod; the TO-FU store is
  the dev default so onboarding works with zero config.
- **Docker check**: `docker --version` + `docker compose version`.
  Missing Docker → install with the official `get.docker.com`
  script (curl | sh, as root via sudo when needed), then
  `systemctl enable --now docker`. Missing Compose plugin only
  → `docker-ce-cli` reinstall is skipped; the script path covers
  it (Compose ships with Docker 23+).
- **Container**: render `deploy/docker-compose.yml` verbatim,
  scp to `~/ferriage/docker-compose.yml` (or `/opt/ferriage/`
  when root), `docker compose pull && docker compose up -d`,
  then gate on `GET /healthz` of the new peer.
- **Mesh link**: the new peer is started with `FERRIAGE_PEER`
  pointing back at the onboarding peer (ADR 0001 is pull-based:
  the new node pulls state from the operator's peer).
- **Destructive-adjacent gate** (AGENTS.md §5): Docker install
  and container start run over an already-authenticated SSH
  session the operator initiated. The peer refuses to onboard
  `localhost` unless `FERRIAGE_ONBOARD_ALLOW_LOCAL=1` (dev
  only). Non-dev hosts always need operator confirmation —
  that confirmation is the request itself, issued by a logged-in
  (Zone-2 trusted) user through the BFF gate.

## Consequences
- `internal/onboard` is the only package that speaks SSH;
  peer API gains `POST /hosts/onboard` (typed, drift-tested).
- The peer image needs no `ssh`/`scp` binaries — it is the SSH
  client.
- First image tag stays Phase-4 user-gated; onboarding works
  against `nopenix/ferriage:<tag>` from the rendered compose
  file (`FERRIAGE_IMAGE` on the compose file, `latest` default).
