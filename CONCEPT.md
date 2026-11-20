# Ferriage — Concept

> **Ferriage** (n.): *the act or business of carrying by ferry.*
> The platform that carries your Docker Compose stacks — and their data —
> between your hosts. One control plane, many shores.

## Problem

Every Docker management product is a single control plane with a single
database. That means: one point of failure, state that forks when you try
to run two of them, no way to move a stack *including its volumes* between
hosts, and a licensing leash if you want the good features. The existing
tools are all the same shape, and all hit the same walls.

Ferriage exists because the category is wrong, not the execution.

## Core idea

**State lives on disk, not in a database.** Compose files in a directory
are the source of truth — git-friendly, fork-proof, inspectable with
`ls`. The control plane is a *view* over that state, not its owner.
Agents on every host execute against the same files. There is no
database to lose, fork, or drift.

**No central control plane — the systems mesh.** Every host runs the same
binary and is a full peer: it manages itself and syncs state with the
others. Peers, not clients. Any single node can be lost without losing
the system — the mesh is the state, not any one machine.

**UI is separate from operations.** The user interface runs as its own
container, distinct from the operational (agent) container on each host.
It is itself a mesh participant: it uses the same peer-to-peer mesh
technique, syncs with the other UI instances, and can be run on any
node. Losing a UI container loses nothing — another one takes over.

## Requirements (hard)

1. **Adopt existing stacks** — discover running Compose projects by Docker
   labels; never force a rewrite of what already runs.
2. **Multi-endpoint** — one view over VPS + home box + LAN host (and beyond).
3. **Move stacks between endpoints, including volumes** — stop → snapshot →
   restore → deploy, orchestrated as one operation. (No product has this.)
4. **Redundant frontends** — multiple UIs over one state. Possible *because*
   state is files; frontends are stateless readers.
5. **Privileged containers first-class** — plain Docker + Compose on every
   host. No swarm, no orchestrator tax.
6. **Compose is the format** — files on disk, never a DB-embedded copy.
7. **OIDC via a standard IdP** (product-agnostic) — one canonical hostname
   per instance; aliases canonicalize at the edge (OIDC sessions bind to
   the entering host).
8. **Coexist with the user's existing reverse proxy** (product-agnostic) —
   no built-in proxy, no port hunger; label-based routing only.
9. **Management survives any single host being down** — the mesh has no
   special node; a dead host degrades to "offline in UI", never
   "management dead".
10. **API-first** — typed API + CLI. Automation is the biggest user.
11. **SSH is the interconnection** — Ferriage connects to hosts over SSH.
    On first contact it checks for Docker and the Compose plugin and
    installs them if missing. **Zero-touch onboarding**: add a host by
    address + key, nothing else.
12. **Self-bootstrapping agent** — the first thing Ferriage does on any
    host it controls is start a single Docker container (with a persistent
    volume) via SSH. That container handles *every* operation Ferriage
    triggers on that host, including establishing mesh connections to
    other SSH endpoints. No additional config required anywhere.

## Requirements (should-have)

- Auto-update (digest poll + redeploy + Shoutrrr), per-stack policy,
  databases excluded by default, snapshot before update.
- **Shoutrrr** notifications (built-in channel) — the versatile one:
  any chat/chatops service, email, webhooks, generic URLs.
- Audit log — who did what, when.
- Log streaming + live stats **+ container console (exec/attach) + a
  proper code editor**: autocomplete, syntax highlighting, compose-file
  validation, diff preview, the full bells and whistles.
- MCP server surface (the homelab is AI-heavy).
- Secrets: per-host env files, file-based secrets, rotation on demand.
- No leaked credentials, API keys over passwords.

## Security model — two zones

**Zone 1 — before login: bulletproof.** The public surface (login page,
OIDC flow, API auth) is hardened to the standard: OIDC-only sessions,
rate limiting, CSRF, strict cookies, no information leakage in errors,
TLS everywhere, no default credentials, no open registration. Anything
reachable without authentication is treated as hostile.

**Zone 2 — after login: trusted users, relaxed security.** Logged-in
users are fully trusted operators — Ferriage is a power tool (it can
run privileged containers and exec into anything), so post-auth
restraint is theater. We do not invest in preventing privilege
escalation between authenticated users: it is assumed to always be
possible, and the defense is the front door, not the interior. Audit
logging records what trusted users did; it does not constrain them.

In short: **harden the gate, trust the room.**

## Backup / Restore / Export (first-class)

- **Incremental backups**: deduplicating, incremental snapshots (restic-
  style) — only changed blocks are stored; retention policies keep
  history without exploding storage. Full snapshots are cheap, so
  back up everything.
- **Per-stack backup**: volumes + bind mounts + overlay dirs + image
  tarballs + compose files + env — one snapshot, restorable from scratch
  onto any node. No control plane needed to restore.
- **Export**: the same snapshot format, portable. Export a stack from
  one host, import it on another — it's just a file, not a migration.
- **Node backup**: whole-host snapshot for disaster recovery (the mesh
  peers make this a per-node operation, not a platform operation).
- **Restore verification**: a restore isn't done until the stack is
  healthy — health-gated, like a deploy.

## Auto-update (distributed)

- Each node runs its own update loop (whale-mate, grown up): digest
  polling per stack, redeploy on change, Shoutrrr on every event.
- Policy per stack: `poll` (report only) vs `auto` (redeploy).
  Databases and stateful services default to `poll`, never `auto`.
- Snapshot-before-update is mandatory for anything with state.

## Architecture essence — the mesh

```
        ┌─────────────────────────────────┐
        │  every node runs the same binary │
        │  (peer — not server/client)      │
        │  + a separate UI container,      │
        │  itself a mesh participant       │
        └─────────────────────────────────┘
          │              │              │
     VPS (peer)    home box (peer)    LAN host (peer)
   stacks on disk  stacks on disk     stacks on disk
        └──── state syncs peer-to-peer (files) ────┘
   Traefik (user's) routes the UI on whichever node you enter.
   Ferriage never touches ports 80/443.
```

- **Peer**: each node manages itself and syncs state with the others.
  State = the stacks directory, synced peer-to-peer (git-style or
  file-sync). No node is special; any node can be lost without losing
  the system.
- **UI container**: separate from the operational container, runs on any
  node, and is itself a mesh participant — same sync technique as the
  peers. Any UI instance sees the whole mesh; losing one loses nothing.
- **Bootstrap**: Ferriage talks to new hosts over SSH only. First thing
  it does there: check Docker + Compose plugin (install if missing),
  then start **one Ferriage container** (persistent volume) on that host.
  That container is the entire agent — it executes every operation
  Ferriage triggers on the host and establishes mesh connections to
  other SSH endpoints. Onboarding a new host = address + SSH key.
- **Mesh sync**: outbound-only connections (NAT-safe, mTLS/Noise-style
  keypairs, no inbound ports). State sync is file-based — the stacks
  dir replicates; volumes move as snapshots (see Backup/Export).
- **Deploy engine philosophy**: imperative, health-gated cutovers
  over declarative reconciliation.
- **OIDC**: one canonical host per instance; the IdP (any OIDC provider)
  is the access gate — everyone it authenticates becomes a user.
- **Transfer**: volume snapshot streamed over the mesh channel
  (or offloaded to restic) → restore → deploy. One operation, not a runbook.

## Component choices — rent, don't build

Only the composition is original. Every layer is someone else's mature
component:

- **Frontend**: shadcn/ui (React + Tailwind, MIT) — components copied into
  the repo, no dependency to break. (Tabler's free MIT core is the
  Bootstrap alternative; PRO is unnecessary.)
- **Mesh**: Iroh (Rust, QUIC + NAT traversal, E2E encrypted, v1.0 stable)
  — `iroh-docs` for CRDT state sync, `iroh-gossip` for peer discovery,
  `iroh-blobs` for volume snapshot transfer. Dial keys, not IPs;
  no port forwarding, no VPN required. (Syncthing is the file-sync
  alternative; LiteFS is the SQLite-replication alternative.)
- **Code editor**: Monaco (MIT) + `monaco-yaml`, pointed at the Docker
  Compose JSON schema → compose-aware autocomplete, diagnostics, hover.
- **Docker**: official Go SDK (`docker/docker/client`) + Compose SDK
  (`compose-spec/compose-go`) — no CLI shelling for the core paths.
- **OIDC**: standard OIDC client library (openidconnect / go-oidc),
  product-agnostic by design.
- **Notifications**: Shoutrrr (Go, MIT) — Slack, Discord, Telegram,
  email, webhooks, generic URLs.
- **Backup**: restic (incremental, deduplicating) — never build a backup
  engine.
- **State metadata**: SQLite (ACID, single-file, syncs as a blob).
  No database server, no DB to lose.

## First milestone (updated)

One peer binary on the home box + one on the VPS, state syncing between
them, adopt one existing stack on each, move it across **with its volume**
via the export/import flow. That single flow proves every hard requirement
(1–6) plus mesh sync and backup/restore. Everything after is breadth.

## Non-goals

- Kubernetes.
- A built-in reverse proxy.
- A proprietary database.
- A license key, a node cap, or a renewal email. Ever.
- Enterprise-style authorization theater (RBAC matrices, per-user
  permission scopes, privilege separation between authenticated users).
  Authenticated users are trusted operators by design.

## Status

Concept stage (2026-11-20). Name verified clear: no product, GitHub org
available, npm/PyPI/Docker Hub/crates.io all 404, domains free
(ferriage.dev / .app / .sh / .codes / .tools).
