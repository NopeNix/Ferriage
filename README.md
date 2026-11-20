# Ferriage

> *Ferriage* (n.): the act or business of carrying by ferry.
> The platform that carries your Docker Compose stacks — and their
> data — between your hosts. One mesh, many shores.

Ferriage is a peer-to-peer Docker Compose fleet manager. There is no
central control plane and no database to lose: Compose files on disk
are the source of truth, every host runs the same binary as a full
peer, and the UI is a separate container that is itself a mesh
participant. Move a stack between hosts — **including its volumes** —
as one operation.

> If you are an AI agent, read **`AGENTS.md`** first. It's the
> operating contract. Then read **`HANDOVER.md`** for current state
> and **`.openhands/memory/MEMORY.md`** for the learnings index.

## Status

Concept stage (2026-11-20). The canonical product spec is
[`CONCEPT.md`](CONCEPT.md) — problem, 12 hard requirements, two-zone
security model, mesh architecture, component choices, and the first
milestone (two peers, state sync, adopt + move one stack with its
volume). **[`ROADMAP.md`](ROADMAP.md)** lays out the phased plan and
the minimum-viable record-keeping discipline for dev iteration.

## Design at a glance

- **State lives on disk, not in a database** — git-friendly,
  fork-proof, inspectable with `ls`.
- **No central control plane** — peers, not clients. Any single node
  can be lost; the mesh is the state.
- **UI separate from operations** — redundant frontends over shared
  file state; losing a UI loses nothing.
- **Rent, don't build** — Go + official Docker/Compose SDKs, Iroh for
  the mesh, shadcn/ui for the front end, restic for backups,
  Shoutrrr for notifications.

## Where things live

| Path | What |
|---|---|
| `CONCEPT.md` | Product spec (canonical) |
| `ROADMAP.md` | Phased plan + low-overhead dev discipline |
| `AGENTS.md` | Operating contract for agents |
| `HANDOVER.md` + `handover/` | Current state + frozen session notes |
| `.openhands/memory/` | Learnings index + per-session logs |
| `docs/` | Architecture, security, deployment, ADRs |
| `scripts/` | `test.sh` (layered tests), `install-hooks.sh` |

Code layout (`cmd/`, `internal/`, `web/`) arrives with the first
implementation milestone — see `docs/architecture.md`.
