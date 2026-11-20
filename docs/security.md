# Security — two zones

Source: [`../CONCEPT.md`](../CONCEPT.md) § "Security model — two
zones". Rule of thumb: **harden the gate, trust the room.**

## Zone 1 — before login: bulletproof

Everything reachable without authentication is treated as hostile.

- **OIDC-only sessions** via any standard IdP (product-agnostic).
  Everyone the IdP authenticates becomes a user. No local passwords,
  no default credentials, no open registration.
- **Rate limiting** on the public surface (login, OIDC flow, API auth).
- **CSRF** tokens on every state-changing request.
- **Strict cookies**: `Secure`, `HttpOnly`, `SameSite` — and correct
  behavior behind proxies (session cookie must survive the
  `Secure`-flagging trap: verify the live instance, don't assume).
- **No information leakage in errors** — generic messages, no stack
  traces, no user enumeration.
- **TLS everywhere.** One canonical hostname per instance; aliases
  canonicalize at the edge (OIDC sessions bind to the entering host).
- **Coexist with the user's existing reverse proxy** — no built-in
  proxy, no port hunger (never 80/443), label-based routing only.

## Zone 2 — after login: trusted operators

- Logged-in users are **fully trusted** — Ferriage runs privileged
  containers and execs into anything, so post-auth restraint is
  theater.
- **Do not build**: RBAC matrices, per-user permission scopes,
  privilege separation between authenticated users. Explicit
  non-goal (see CONCEPT.md).
- **Audit log** records who did what, when — it observes trusted
  users; it does not constrain them.

## Secrets hygiene

- Per-host env files, file-based secrets, rotation on demand.
- **API keys over passwords.** No leaked credentials, ever.
- No secrets in `HANDOVER.md`, `.openhands/memory/`, ADRs, or any
  committed file. Local secret files go in `.gitignore` / data dirs
  that are never committed (see `AGENTS.md` §8).

## Agent checklist (Zone 1 changes)

Before shipping anything touching the pre-login surface:

- [ ] Rate limiting present on new endpoints?
- [ ] CSRF token required on new POSTs?
- [ ] Cookies `Secure` + `HttpOnly` + `SameSite`?
- [ ] Errors generic (no internals leaked)?
- [ ] Verified live in a browser (not just curl)? (§2 visual-first)
