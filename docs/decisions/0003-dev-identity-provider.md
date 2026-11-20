---
status: accepted
date: 2026-11-20
---

# 0003 — Dev identity provider for Phase 2: Dex (not Zitadel)

## Context

Phase 2 needs a Zone-1 OIDC gate (`docs/security.md`): a dev IdP to
exercise the login flow on the home box. Two candidates were
evaluated:

- **Zitadel** (the operator's default preference): single-container
  OIDC, but v4 requires a CockroachDB store + a generated masterkey
  — two more stateful things on the dev box, and the masterkey
  ceremony is pure friction for a dev-only stand-in.
- **Dex**: single container, static YAML config, sqlite storage
  (or in-memory), standard OIDC/PKCE, no DB.

Both are standard OIDC providers; the product is IdP-agnostic
(CONCEPT req. 7, `docs/security.md`). The dev choice does not
constrain prod: the BFF speaks OIDC discovery and ships any
issuer string, so swapping Dex for Zitadel/Keycloak/Authentik
later is config-only.

## Decision

**Dex `v2.41.1`, dev-only, via `deploy/dev/dex.yml`.**

- Runs on `--network host` at `http://localhost:18081`, issuer
  `http://localhost:18081` (root path — Dex serves discovery at
  `/.well-known/openid-configuration`; a `/dex` path issuer
  breaks `openid-client`'s discovery URL composition).
- One static client `ferriage-web`, redirect
  `http://localhost:19080/auth/callback` (dev port; prod uses the
  canonical hostname per `docs/deployment.md`).
- Password DB enabled with one static user
  `operator@ferriage.dev` (bcrypt hash, cost 12). Plaintext
  `devpassword` — **dev only, never prod** (security.md: no
  default credentials; this is a local stand-in, rotated with
  `htpasswd`-style regeneration).
- `skipEmailVerification: true` (static passwords have no inbox).
- `webserver: false` (no gRPC — we don't need the API).

## Consequences

- Phase 2.2 E2E (login → password → approval → callback →
  session cookie → protected `/api`) is proven against Dex.
- Prod IdP is a **separate decision** (Phase 4 ship): any OIDC
  provider works; no code change expected.
- The BFF (`web/bff/auth.mjs`) allows plaintext HTTP **only**
  when the issuer is `http://localhost` (dev); prod issuers must
  be `https://` (security.md: TLS everywhere).
- Bcrypt example hash from Dex docs is invalid ("hashedSecret
  too short") — generate real hashes (e.g. with python `bcrypt`).
