# Deployment

## Hosts

| Host | Role | Notes |
|---|---|---|
| Home box | **DEV** — prototyping, agent iteration | Edit → rebuild → run here → screenshot |
| VPS | **PROD** | First-milestone peer. Never test mesh/agent changes without confirmation |
| LAN host(s) | **PROD** | Additional peers |

Dev vs prod discipline: `AGENTS.md` §5. State which host a change was
tested on in `HANDOVER.md`.

## SSH onboarding (zero-touch)

1. Add a host by **address + SSH key** — nothing else.
2. First contact over SSH: check Docker + Compose plugin, install
   if missing.
3. Start **one Ferriage container** (persistent volume) on the host.
   That container is the entire agent and establishes mesh
   connections to other endpoints.

⚠ Step 2 installs Docker on the target — destructive-adjacent.
Confirm with the operator before running against any host that
isn't the dev box.

## Reverse proxy coexistence

- Ferriage ships **no built-in reverse proxy** and never binds
  80/443. The user's existing proxy (e.g. Traefik) routes the UI
  on whichever node you enter, via labels.
- **OIDC**: one canonical hostname per instance; aliases
  canonicalize at the edge (OIDC sessions bind to the entering host).

## Mesh networking

- Outbound-only connections, NAT-safe, keypair-based identity
  (dial keys, not IPs). No inbound ports, no port forwarding,
  no VPN required.

## Images & releases

- GitHub home: [`NopeNix/Ferriage`](https://github.com/NopeNix/Ferriage)
  (public, confirmed 2026-11-20).
- Docker Hub: **`nopenix/ferriage`** (confirmed 2026-11-20).
  First image tag: record here when the first image ships.
- Release rules (tag / release / image push need explicit
  confirmation): `AGENTS.md` §6.

## Backup / restore (first-class)

- Incremental, deduplicating snapshots (restic-style).
- Per-stack backup: volumes + bind mounts + overlay dirs + image
  tarballs + compose files + env — restorable onto any node.
- Export/import = the same snapshot format as a portable file.
- Restore is **health-gated** — not done until the stack is healthy.

## Auto-update (distributed)

- Each node runs its own update loop: digest poll per stack,
  redeploy on change, Shoutrrr on every event.
- Policy per stack: `poll` (report only) vs `auto` (redeploy).
  Databases/stateful services default to `poll`, never `auto`.
- Snapshot-before-update is mandatory for anything with state.
