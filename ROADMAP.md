# Ferriage — Roadmap

Canonical spec: [`CONCEPT.md`](CONCEPT.md). Operating contract:
[`AGENTS.md`](AGENTS.md). **Goal of this file:** make dev iteration
cheap — the fastest path from idea to working code on the home box,
with the least paperwork per step. Records exist to unblock the
*next* session, not to document this one.

> **Low-record principle:** if a step doesn't change what the next
> agent must know, don't write it down. One line in `HANDOVER.md` >
> a page in `handover/` > nothing. Detail only for root causes,
> decisions, and expensive-to-rediscover facts.

## Dev loop (the whole game)

```
edit (home box) → scripts/test.sh --fast → docker compose up (1 peer, local)
→ browser (real Chrome, live instance) → screenshot if UI touched
```

- **Home box only** for prototyping. VPS/LAN = prod, never test
  mesh changes against them without confirmation (AGENTS.md §5).
- **No commits between micro-iterations.** Commit when the change is
  done and `--fast` is green — not at every save.
- **Point at the live URL from the instance you changed** (never
  from memory). Record it in `HANDOVER.md`.

## Phase 0 — Decisions (1 session, blocks everything)

Confirm the stack so scaffolding doesn't get re-litigated:

- [x] Go peer + Docker (`docker/docker/client`) + Compose
      (`compose-spec/compose-go`) SDKs (confirmed 2026-11-20)
- [x] React + shadcn/ui UI (separate container) (confirmed 2026-11-20)
- [x] Iroh mesh (`iroh-docs` / `iroh-gossip` / `iroh-blobs`)
      (confirmed 2026-11-20)
- [x] GitHub home: `NopeNix/Ferriage` (confirmed 2026-11-20);
      Docker Hub: `nopenix/ferriage` (confirmed 2026-11-20);
      first image tag recorded in `docs/deployment.md` when shipped
- [x] Dev = home box, prod = VPS + LAN hosts (confirmed)

**Exit:** stack confirmed in `HANDOVER.md`; ADR only if a choice
deviates from CONCEPT.

## Phase 1 — First milestone: two peers, one stack, moved with volume

The milestone that proves the category (CONCEPT § "First milestone").

| Step | Deliverable | Prove it |
|------|-------------|----------|
| 1.1 | `cmd/`, `internal/`, `go.mod` skeleton; peer binary starts, serves health endpoint | `scripts/test.sh --fast` |
| 1.2 | State dir: stacks on disk; peer lists stacks from labels (adopt existing) | `docker compose ps` untouched stacks appear |
| 1.3 | Mesh sync: two peers (home box + VPS) replicate state dir | file added on A appears on B (✓ 2026-11-20, ADR 0001) |
| 1.4 | Export: snapshot stack + **volumes** → portable file | round-trip restore on same host (✓ 2026-11-20, ADR 0002) |
| 1.5 | Import/move: restore snapshot on other peer → deploy → **health-gated** (✓ 2026-11-20) | stack healthy on B, data intact |
| 1.6 | `deploy/docker-compose.yml` for Ferriage itself; layer-2 smoke in `scripts/test.sh` turns green (✓ 2026-11-20) | `scripts/test.sh` (full) |

**Exit:** one existing stack adopted on each host, moved across with
its volume, healthy on arrival. Record host + URL in `HANDOVER.md`.

## Phase 2 — UI shell (first pixels) ✓ 2026-11-20

- [x] Minimal React/shadcn UI container — stack list, peer status
  ("offline in UI" for dead hosts). Stack detail is Phase 3.
- [x] Zone-1 auth gate: OIDC-only login in the BFF (rate limit,
  CSRF, strict cookies) — ADR 0004. Dev IdP: Dex (ADR 0003).
- [x] Playwright baseline: `tests/baselines/` + png-diff in
  `scripts/test.sh` layer 3 (scripts/png-diff.mjs).

**Exit:** live UI in a real browser, screenshot baseline committed.

## Phase 3 — Breadth (in priority order)

1. **API-first + CLI** — typed API, every UI action has a CLI twin ✓ 2026-11-20
2. **Logs / stats / exec** — stream, stats, container console,
   Monaco editor (compose schema autocomplete + validation + diff) ✓ 2026-11-20 (ADR 0005)
3. **SSH onboarding** — zero-touch: address + key → Docker check →
   one Ferriage container (confirm before non-dev hosts) ✓ 2026-11-20 (ADR 0006)
4. **Notifications** — Shoutrrr (built-in channel) ✓ 2026-11-20 (ADR 0007)
5. **Audit log** — who did what, when ✓ 2026-11-20 (ADR 0008)
6. **Auto-update** — per-node digest poll, `poll` vs `auto` policy,
   snapshot-before-update, DBs default `poll` ✓ 2026-11-20 (ADR 0009)
7. **MCP server** — homelab is AI-heavy
8. **Backup/restore polish** — incremental restic snapshots,
   retention, node-level disaster recovery

## Phase 4 — Ship (only when user says so)

Tag → release → push image (AGENTS.md §6). Never push/tag/release
without explicit confirmation.

## Record-keeping (minimum viable)

| When | File | What |
|------|------|------|
| Every session end | `HANDOVER.md` | Rewrite `~40` lines: where work is, next step, host, live URL |
| Only if you learned something expensive | `.openhands/memory/log/<date>-<nnn>-<slug>.md` + index row | Root cause / quirk / decision — not a diary |
| Only if a decision sticks | `docs/decisions/NNNN-<slug>.md` | Wire/sync format, Zone-1, release tag changes |
| Feature/change ships | `git commit` | `--fast` green first; update `HANDOVER.md` in same commit |

**Don't record:** directory listings, obvious commands, per-step
narratives, anything re-discoverable with `ls`/`grep`.

## Open questions (live — answer these first)

1. ~~Docker Hub namespace?~~ — `nopenix/ferriage` (confirmed 2026-11-20).
   First image tag still to record in `docs/deployment.md` at ship time.
2. ~~IdP for dev OIDC?~~ — Dex (ADR 0003), dev-only. Prod IdP
   is a separate Phase 4 decision.
3. ~~State-sync mechanism: git-style vs file-sync~~ — decided
   2026-11-20: manifest-based file sync (ADR 0001).
