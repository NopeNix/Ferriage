---
date: 2026-11-20
session: 001
slug: harness-setup
topics: [harness, memory, handover]
---

# Session 001 — harness setup

## Learnings / decisions

- **Index + single files beat monoliths** for memory and handover:
  `MEMORY.md` / `HANDOVER.md` are indexes only; one file per session
  under `log/` / `handover/`. New session = new file, never append.
- **Memory lives in `.openhands/memory/`**, not root `memory/` — the
  platform auto-injects `.openhands/memory/MEMORY.md` at session start;
  a root `memory/MEMORY.md` would be invisible to the next agent.
- **Both trees are committed** (project knowledge). Redact secrets
  from text; exclude only true scratch (`tmp/`, `data/`, `stacks/`)
  via `.gitignore` / `.git/info/exclude`. Differs from nopenix, which
  excluded `STATUS-session.txt` — Ferriage wants handover visible
  to a fresh clone.
- **Ferriage harness ≈ nopenix.de shape, Ferriage content**: kept
  read order, question budget, release contract (extended to Docker
  image pushes), layered `test.sh` + pre-commit hook, after-action
  checklist. Dropped: htmx/CSS/module rules, uv/pytest/mypy, printer
  and proxy specifics.

## Gotchas

- None yet — no code. First real gotcha goes here (new file, new row).
