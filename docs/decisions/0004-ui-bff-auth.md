---
status: accepted
date: 2026-11-20
---

# 0004 — Zone-1 auth lives in the UI BFF, not the peer

## Context

Phase 2 needs the Zone-1 gate (`docs/security.md`): OIDC-only
sessions, rate limiting, CSRF, strict cookies. Two shapes were
considered:

- **Peer-enforced**: the peer binary validates sessions itself
  (OIDC client in Go, `coreos/go-oidc`); the UI is a static SPA.
- **BFF-enforced**: the UI container runs a tiny backend-for-
  frontend that holds the OIDC session and proxies `/api/*` to
  the peer. The peer stays plaintext on the trusted network.

## Decision

**BFF-enforced.**

- UI container = `node:24-alpine` serving the built SPA from
  `web/dist` + `web/bff/index.mjs` proxy. `/api/*` → peer
  `FERRIAGE_PEER` (default `http://localhost:8080`).
- `/auth/*` (BFF-owned): `login` → `callback` (OIDC authorization
  code + PKCE, `openid-client` v6) → HMAC-signed session cookie
  (`ferriage_session`, `HttpOnly` + `SameSite=Lax` + `Secure` when
  TLS) → `me` (session identity + CSRF token for the SPA) →
  `logout`.
- Every `/api/*` request checks: session valid **and**, for
  non-GET, `X-CSRF-Token` matches the session's token
  (constant-time). Rate limits: login 10/min, callback 20/min,
  API 600/min per IP.
- No session store: the cookie is self-contained (HMAC, 8h
  expiry). Losing a UI container loses nothing — another one
  takes over (CONCEPT).
- Peer unchanged: still serves plaintext HTTP on the trusted
  network; TLS is the reverse proxy's job (CONCEPT req. 8).

## Consequences

- The peer can be swapped / reimplemented without touching auth;
  the BFF is the only Zone-1 code.
- Dev mode: with `FERRIAGE_OIDC_ISSUER` unset the BFF proxies
  without auth (home-box prototyping only; never prod).
- Phase 3's CLI/MCP surfaces will need their own auth story
  (API keys over passwords, per security.md) — they arrive with
  the typed API, not here.
