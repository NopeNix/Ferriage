---
date: 2026-11-20
session: 014
slug: phase3-ssh-onboarding
topics: [phase3, ssh, onboarding, adr-0006, e2e]
---

# Session 014 — Phase 3 (3/8): SSH onboarding

Full routine **40/0**. ADR 0006: native `golang.org/x/crypto/ssh`
(no openssh-client in the image), key-only auth, host keys via
`FERRIAGE_SSH_KNOWN_HOSTS` pin file or TO-FU store in
`<state>/.ferriage/known_hosts`, Docker install via get.docker.com,
compose rendered from a template (not the repo file).

- `internal/onboard`: Dial (key-only), Exec/Upload (`cat > path`,
  no SFTP), CheckDocker/EnsureDocker, RenderCompose, Onboard
  (Runner interface seam for tests), WaitHealthy.
- Peer: `POST /hosts/onboard` (typed, drift-tested), handler
  returns 502 + partial steps on failure so the UI shows progress.
  `FERRIAGE_SELF_URL` = the URL new peers pull from.
- CLI: `ferriage onboard <addr> --key <file> [--user] [--image]
  [--peer-url] [--port]`.
- UI: `OnboardHost` card (address+key form, step list); BFF needed
  no change (generic /api proxy). Stack-list baseline regenerated
  (OnboardHost card is the intended layout change).

Expensive-to-rediscover bugs found live:

- **TO-FU store existence check**: a missing/empty store file
  must mean TO-FU, not "pinned, key not found". Only a non-empty
  store counts as pinned. Tested e2e via `ssh-keygen`'s host key
  flow on the dev box sshd.
- **`pull_policy: always`** re-pulls the image and hides that a
  locally-built image works — use `missing` (falls back to local
  cache, pulls only when absent). Bit us twice (first with a
  bare `ferriage:` tag that Docker resolved to Docker Hub).
- **linuxserver/openssh-server** container: user is
  `linuxserver.io`, and `-p 2222:22` + fail2ban-ish resets made
  loopback SSH unreliable; the dev box's own sshd (root-owned,
  already running) is the reliable e2e target here.

E2E proof (dev host): peer on :18080, `POST /hosts/onboard`
`127.0.0.1` as `user` with the dev key → steps ssh/docker-check/
compose-render/container-start/healthz all ✓, new peer healthy on
:18085 with own peer_id, known_hosts written. CLI twin re-ran the
same flow green. Cleaned up: onboarded container, ~/ferriage,
test authorized_keys lines, local test image tags.
