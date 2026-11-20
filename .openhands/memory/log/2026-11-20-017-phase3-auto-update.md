---
date: 2026-11-20
session: 017
slug: phase3-auto-update
topics: [phase3, auto-update, docker, shoutrrr, digest]
---

# Session 017 — auto-update lessons

- **Docker digest compare traps**: `ImageInspect.ID` (config hash)
  and `RepoDigests[0]` (manifest hash) are different hash families;
  comparing ID vs `DistributionInspect.Descriptor.Digest` is always
  "available". Use `RepoDigests[0]` as local, manifest digest as
  remote. Pinned (`@sha256:`) and locally-built (no RepoDigest)
  images are *unresolvable*, never up-to-date.
- **Compose render drops unreferenced volumes**: `docker compose
  config` omits a top-level `volumes:` entry no service mounts.
  Stateful heuristic must check the rendered model (top-level
  `volumes` map OR service volume `type: "volume"`), with a text
  fallback that only counts top-level `volumes:`.
- **Shoutrrr generic webhook URL**: `generic://host/path?disabletls=yes`
  for http; `generic://http://…` parses the scheme wrong and
  dials `https://http//…`.
- **Stale dev peer on :18082**: a session-016 peer survived
  (child of a `go run`); it kept serving the old binary, so
  `/update` 404'd and the audit log was empty on a fresh state
  dir. Kill by PID via `ss -ltnp`, not by pattern; `go run`
  leaves the exe in `go-build` cache — kill the exe too.
- **Full routine now 46/0** (was 40/0 pre-update; +6 = gen:check,
  web tests, CLI e2e additions).
