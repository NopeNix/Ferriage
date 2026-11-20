# AGENTS.md — Read this FIRST, every session

You are an LLM agent working in the **`Ferriage`** repo — a peer-to-peer
Docker Compose fleet manager. State lives on disk (Compose files), not in
a database; every host runs the same binary as a full peer; the UI is a
separate container that is itself a mesh participant.

> **The contract in one sentence**: state is files, peers are peers,
> harden the gate and trust the room, see it in a real browser, ship the
> smallest change that solves the problem — and never push or release
> without the operator saying so.

Read this file end-to-end **before changing any code or documentation**.
It is the operating contract. `CONCEPT.md` is the product spec;
`docs/` covers architecture, security, and deployment in detail.

## 1. Read order (mandatory)

When the user opens a new session, ask no questions, write no code,
push no buttons. Do this first:

1. **Read `HANDOVER.md`** — current state, where work is, what's next.
   It is rewritten each session, so it is always current.
2. **Read `.openhands/memory/MEMORY.md`** — the memory index. Follow
   links into `log/*` only for topics relevant to today's task.
3. **Read this file (`AGENTS.md`)** end-to-end. Do not skim.
4. **Read `CONCEPT.md`** — the product spec. Requirements, architecture
   essence, non-goals, and component choices all live there.
5. **Read the relevant `docs/` file** for the surface you will touch:
   architecture → mesh & state; security → two-zone model; deployment →
   SSH onboarding & hosts. Read `docs/decisions/` for past decisions
   before re-litigating one.
6. **If the user names a live URL**, immediately confirm it is live
   (`curl -sSL <url> | head`) and that the origin matches. Don't claim
   a URL is live without touching it.

**Do not invent.** If something is unclear, say so and ask. If you'd
like to deviate from a convention, surface the deviation explicitly
and ask for sign-off — not silently.

## 2. The visual-first contract

Ferriage has a real UI (shadcn/ui + React + Tailwind, running as its own
container). Pixels are the product.

- **Before any UI change**, open the live instance in a real browser
  (Chrome / Firefox — not curl). Navigate the affected flows, take a
  screenshot, compare against baselines under `tests/baselines/`.
- **Write the test plan first**: "I will capture baseline PNG X, run
  the visual diff at Y% threshold." Don't write code until you can
  articulate that.
- **Visual diffs are PNG-against-PNG**, not DOM/bounding-box
  assertions. Computed styles don't ship; pixels do.
- **Re-screenshot after every change.**
- **Trust the user's report.** If the user says something isn't showing
  and your DOM check says it is, the user is right. Reconcile against
  their report; don't argue with `outerHTML`.

## 3. Mesh-first contract

The load-bearing rule of the whole project (see `CONCEPT.md`):

- **State is files, never a database.** Compose files in a directory are
  the source of truth. Never introduce a database as owner of state.
  Any metadata store (SQLite) is a derived view that syncs as a blob
  and can be rebuilt from the files.
- **Peers, not clients.** Every node runs the same binary, manages
  itself, and syncs state with others. No special node, no control
  plane. A dead host degrades to "offline in UI", never "management
  dead" — if your change breaks this, it is wrong.
- **UI is a mesh participant, not a backend.** The UI container syncs
  like any peer and can run on any node. Losing a UI container loses
  nothing. Never make the UI the only path to state.
- **Wire/format compatibility is sacred.** State-sync format, snapshot
  format, and node identity changes need a migration note and a
  decided ADR (see `docs/decisions/`) before implementation.
- **Adopt, don't rewrite.** Discover existing stacks by Docker labels.
  Never force a rewrite of what already runs.

## 4. Two-zone security

From `CONCEPT.md` — **harden the gate, trust the room**:

- **Zone 1 (before login): bulletproof.** OIDC-only sessions, rate
  limiting, CSRF, strict cookies, no information leakage in errors,
  TLS everywhere, no default credentials, no open registration.
  Anything reachable without authentication is treated as hostile.
- **Zone 2 (after login): trusted operators.** Logged-in users are
  fully trusted (Ferriage can run privileged containers and exec into
  anything). Do **not** invest in RBAC matrices, per-user permission
  scopes, or privilege separation between authenticated users — that's
  theater and is an explicit non-goal.
- **Audit log** records what trusted users did; it does not constrain them.
- **No leaked credentials.** API keys over passwords. Secrets are
  per-host env files / file-based secrets, rotated on demand.

## 5. Host + iteration contract

Two kinds of hosts — never confuse them:

- **DEV — home box.** The operator's dev / prototyping host. Iterate
  here: edit, rebuild the peer image, run it on the home box, watch it
  in the browser. This is where agent work happens.
- **PROD — VPS + LAN hosts.** Real stacks, real data. Do not test
  agent/mesh changes against these without explicit confirmation.

**SSH onboarding is destructive-adjacent** — it checks for and installs
Docker + the Compose plugin on a host. Confirm with the operator before
running it against any host that isn't the dev box.

Point users at a URL from the instance you actually changed — never
from memory. State in `HANDOVER.md` which host a change was tested on.

**Fast loop during prototyping:** edit → rebuild → run on home box →
screenshot. No commits/pushes between micro-iterations, and no full
`scripts/test.sh` run — that's for "ship it" moments. Full rule: §9.

## 6. Release & version-control contract

This is non-negotiable:

- **Never push to `main` without explicit confirmation** from the user.
- **Never tag, `gh release create`, or push a Docker image without
  explicit confirmation.** Tagging creates a permanent artifact on
  GitHub and Docker Hub; retagging `latest` is a release.
- **The pre-release side**: branch, commit, push feature branch, PR,
  review, merge. **The post-release side**: tag, release, push image.
  **No shortcuts.**
- Repo/registry home (from `CONCEPT.md`): GitHub org `NopeNix`,
  Docker Hub namespace to be confirmed in `docs/deployment.md` once
  the first image ships.

## 7. The question budget

- Default to **one question per turn**. If you need more, bundle them
  into a single multi-question list.
- If a question is **dismissed with no answer**, pick the safer /
  more conservative default and proceed. Don't re-ask.
- If the user says "go" / "do it" / "go for it" without picking a
  specific option, that's a green light to use your best judgement.
  Don't stop to clarify.
- **Preferences that should always be asked before code**:
  - Anything that changes mesh/state-sync/snapshot wire formats
  - Anything that touches Zone 1 (pre-login) security
  - Whether a change ships as a new image tag or modifies an existing one

## 8. Memory & handover — index + single files

Memory and session notes are **never monoliths**. Both are an index
plus many small files, so the next agent reads the index and follows
only the links it needs.

### Memory (`.openhands/memory/`)

- `MEMORY.md` — **index only**. A telegraphic table (date, link, topic,
  takeaway). Keep it under ~6000 chars — the platform truncates older
  content first, so it must stay lean. Never paste prose into it.
- `log/<date>-<nnn>-<slug>.md` — one file per session/topic.
  **New session = new file.** Never append to or rewrite a finished
  log file (typo fixes excepted). Growth happens sideways.
- **Update the index in the same commit** as the new file. An
  unindexed file is invisible.
- At ~100 entries, archive the oldest table rows into
  `log/archive/INDEX-<year>.md` and keep the live index lean.

### Handover (`HANDOVER.md` + `handover/`)

- `HANDOVER.md` — **current state only** (~40 lines max), rewritten at
  the end of each session: where work is, next session, dev host,
  live instance, and a one-line-per-session log linking to detail.
- `handover/<date>-<nnn>-<slug>.md` — frozen per-session detail:
  what was done, operator asks, test state, screenshots, next steps.
- Both are **committed**. They are project knowledge (like d2s
  `MEMORY.md`), not local scratch — and must be visible to a fresh
  clone. Redact host IPs / passwords from the text instead of
  excluding files. Only truly local scratch (`tmp/`, `data/`,
  `stacks/`) goes in `.git/info/exclude`.

### Log file header (both trees)

Every session/log file starts with:

```md
---
date: YYYY-MM-DD
session: NNN
slug: short-slug
topics: [topic, ...]
---
```

so `grep` across the directory works like a second index.

### Hygiene rules

- **No secrets in any of it.** Ever.
- **No trivially re-discoverable facts** (directory listings, obvious
  commands). Record what was expensive to learn: root causes,
  environment quirks, decisions and their reasons.
- Durable decisions also get an ADR: `docs/decisions/README.md` is
  the index, `docs/decisions/NNNN-<slug>.md` are single files.

## 9. Testing & verification contract

Every code change MUST pass layer 1; layers 2–3 are for UI changes and
"ship it" moments:

```sh
scripts/test.sh --fast    # layer 1: lint + unit tests (every change)
scripts/test.sh           # layers 1-3: + compose smoke + browser/visual
```

- **Layer 1** — gofmt, `go vet`, `golangci-lint` (once configured),
  `go test ./...`, frontend lint/tests once the UI exists.
- **Layer 2** — compose smoke: build the peer image, run **two**
  containers, verify state sync, adopt a test stack, move it
  **with its volume** via export/import. This is the first-milestone
  flow; if it breaks, nothing else matters.
- **Layer 3 (UI only)** — Playwright PNG baselines, before/after.

The pre-commit hook (mandatory — §10) runs layer 1. If the repo lacks
test infrastructure for a change, say so and ask before investing time
in building it.

## 10. After-action checklist (every commit)

0. **Install the pre-commit hook** — `bash scripts/install-hooks.sh`
   (idempotent). Do this before your first commit in a fresh clone.
1. `scripts/test.sh --fast`
2. **For UI changes**: baseline screenshot before and after, paste
   both into the chat, describe the diff. If a baseline disagrees with
   your screenshot, the screenshot wins — regenerate after sign-off.
3. `git commit` (the pre-commit hook runs layer 1).
4. **Update memory/handover** per §8 — new log file + index row, in
   the same commit.
5. **Stop.** Don't push. "Looks good to me" only means *you* can't see
   a problem. The user gets the next say.

When the user signals ship: confirm (§6), then tag → release → push image.

## 11. Layout

```
CONCEPT.md            # product spec (canonical)
AGENTS.md             # this file — operating contract
README.md             # product intro; points agents here
HANDOVER.md           # current state (index); handover/ = frozen sessions
.openhands/memory/    # MEMORY.md index + log/ = per-session learnings
docs/
  architecture.md     # mesh, state-on-disk, peer binary, UI container
  security.md         # two zones; Zone 1 checklist
  deployment.md       # SSH onboarding, Docker bootstrap, Traefik rules
  decisions/          # ADRs: README.md index + NNNN-<slug>.md files
scripts/
  test.sh             # layered test routine (layer 1 = --fast)
  install-hooks.sh    # pre-commit hook installer
# (code layout — internal/, cmd/, web/ etc. — lands with the first
#  implementation milestone; see docs/architecture.md)
```

## 12. Commands

```sh
scripts/test.sh --fast          # layer 1 (lint + unit) — every change
scripts/test.sh                 # layers 1-3 (full routine) — "ship it"
bash scripts/install-hooks.sh   # install pre-commit hook (first clone)

# Go (once go.mod lands):
go vet ./... && go test ./...   # must stay green; test.sh wraps these
```
