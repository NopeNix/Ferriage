---
date: 2026-11-20
session: 001
slug: harness-setup
topics: [harness, memory, handover, agents]
---

# Session 001 — harness setup

Set up the Ferriage AI harness, modeled on `~/nopenix.de` but adapted:
dropped the Python/htmx/module machinery, kept the operating-contract
shape, question budget, release contract, and handover discipline.

## What landed

- `AGENTS.md` — read order, visual-first, mesh-first, two-zone
  security, host/iteration contract, release contract, question
  budget, memory/handover rules, layered testing, after-action
  checklist.
- `README.md` — product intro, points agents at AGENTS.md.
- `HANDOVER.md` (index) + `handover/` (frozen sessions).
- `.openhands/memory/MEMORY.md` (index) + `.openhands/memory/log/`
  (per-session learnings). Chosen over root `memory/` because the
  platform auto-injects `.openhands/memory/MEMORY.md` at session start.
- `docs/architecture.md`, `docs/security.md`, `docs/deployment.md`,
  `docs/decisions/README.md` (ADR index + format).
- `scripts/test.sh` (layered: fast = lint+unit; full adds compose
  smoke + Playwright) and `scripts/install-hooks.sh`.
- `.gitignore` + `.git/info/exclude` for local scratch (`tmp/`,
  `data/`, `stacks/`).

## Key decisions

- Memory/handover are **index + single files**, not monoliths
  (operator call). `HANDOVER.md` / `MEMORY.md` are indexes only;
  detail is one file per session, updated sideways, never appended to.
- Both trees are **committed** (project knowledge, like d2s
  `MEMORY.md`) — redact secrets from text rather than excluding files.
  nopenix excluded `STATUS-session.txt`; Ferriage deliberately doesn't.

## Open questions for next session

1. Confirm stack: Go peer + Docker/Compose SDKs, React/shadcn UI,
   Iroh mesh (per CONCEPT) — any changes?
2. GitHub home: `NopeNix/Ferriage`? Docker Hub namespace?
3. Confirm dev = home box, prod = VPS + LAN hosts.
4. Initial commit: harness + CONCEPT together (proposed YES).

## Test state

`scripts/test.sh --fast` passes (no code yet — Go/frontend layers
skip cleanly). No live instance, no screenshots (no UI yet).
