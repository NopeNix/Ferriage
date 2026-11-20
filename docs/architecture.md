# Architecture

Canonical product spec: [`../CONCEPT.md`](../CONCEPT.md). This file is
the agent-facing summary — read it before touching mesh, state, or
sync code (see `AGENTS.md` §3, mesh-first contract).

## The mesh

- Every node runs the **same binary** and is a full peer: it manages
  itself and syncs state with the others. No server/client split,
  no special node.
- The **UI is a separate container**, itself a mesh participant —
  same sync technique as peers. Any UI instance sees the whole
  mesh; losing one loses nothing.
- **Mesh transport**: outbound-only connections (NAT-safe),
  mTLS/Noise-style keypairs, no inbound ports. Dial keys, not IPs.
- Component choice (per CONCEPT): **Iroh** (Rust, QUIC + NAT
  traversal) — `iroh-docs` for CRDT state sync, `iroh-gossip` for
  peer discovery, `iroh-blobs` for volume snapshot transfer.

## State

- **Compose files in a directory are the source of truth** —
  git-friendly, fork-proof, inspectable with `ls`. The control
  plane is a *view* over that state, never its owner.
- State sync = file-based replication of the stacks directory
  (git-style or file-sync). Volumes move as snapshots
  (see Backup/Export in CONCEPT), never as live mounts.
- **SQLite** is allowed only as derived metadata (ACID, single
  file, syncs as a blob) — never as source of truth.
- **Deploy philosophy**: imperative, health-gated cutovers over
  declarative reconciliation. A restore/deploy isn't done until
  the stack is healthy.

## Bootstrap & onboarding

- Ferriage talks to new hosts over **SSH only**. First contact:
  check Docker + Compose plugin (install if missing), then start
  **one Ferriage container** (persistent volume) on that host.
- That container is the entire agent: it executes every operation
  Ferriage triggers on the host and establishes mesh connections
  to other SSH endpoints.
- Onboarding a new host = **address + SSH key**. Nothing else.
  (Destructive-adjacent — confirm before running against any host
  that isn't the dev box; see `AGENTS.md` §5.)

## Transfer (the headline flow)

Volume snapshot streamed over the mesh channel (or offloaded to
**restic**) → restore → deploy. One operation, not a runbook.
Proves the first milestone: two peers (home box + VPS), state
syncing, adopt one existing stack on each, move it across
**with its volume** via export/import.

## Component choices (rent, don't build)

| Layer | Choice |
|---|---|
| Frontend | shadcn/ui (React + Tailwind, MIT) |
| Mesh | Iroh (`iroh-docs` / `iroh-gossip` / `iroh-blobs`) |
| Code editor | Monaco + `monaco-yaml`, Compose JSON schema |
| Docker | Go SDK (`docker/docker/client`) + `compose-spec/compose-go` |
| OIDC | Standard OIDC client (product-agnostic) |
| Notifications | Shoutrrr (Go, MIT) |
| Backup | restic (never build a backup engine) |
| State metadata | SQLite (derived only) |

## Code layout (planned — lands with first milestone)

```
cmd/          # peer binary entrypoint(s)
internal/     # Go packages: mesh, state, deploy, transfer, update, api
web/          # React/shadcn UI (separate container)
deploy/       # compose files for the Ferriage stack itself
tests/        # Go tests + Playwright baselines (tests/baselines/)
```

Do not create this skeleton speculatively — it appears with the
first implementation change, per `AGENTS.md` §10.
