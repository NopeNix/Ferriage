---
date: 2026-11-20
session: 004
slug: phase1-scaffold
topics: [phase1, scaffold, go-toolchain]
---

# Session 004 — Phase 0 confirmed done, Phase 1.1 scaffold

Re-verified Phase 0 exit (HANDOVER + ROADMAP + CONCEPT + session 003):
stack confirmed as-is (Go + Docker/Compose SDKs, React/shadcn UI,
Iroh mesh), repos confirmed (GitHub NopeNix/Ferriage, Docker Hub
nopenix/ferriage), no deviations → no ADR. Two items consciously
deferred: dev IdP (Phase 2) and state-sync mechanism (ADR before
Phase 1.3).

Phase 1.1 done: `go.mod` (module github.com/NopeNix/Ferriage, go 1.25),
`cmd/peer/main.go` (binary starts, graceful shutdown, FERRIAGE_ADDR),
`internal/health` (GET /healthz → `{"status":"ok","version","commit"}`),
`internal/version` (ldflags-overridable, defaults dev/unknown),
`.golangci.yml` (errcheck/govet/ineffassign/staticcheck/unused).
`scripts/test.sh --fast`: 4 passed, 0 failed. Live curl on :18080
verified. Pre-commit hook installed.

Quirk: no sudo on dev host (password required, `sudo -n` fails) so
apt/snap Go is unavailable to the agent. Installed Go 1.25.5
official tarball to `~/.local/go` + golangci-lint 2.6.2, symlinked
`go`/`gofmt`/`golangci-lint` into `~/bin` (on PATH via ~/.bashrc
`export PATH=~/bin:$PATH`). Reuse this — don't retry apt/sudo.
