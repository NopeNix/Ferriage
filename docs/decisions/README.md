# Decision records (ADRs)

One short file per decision: `NNNN-<slug>.md`, numbered
chronologically. This file is the **index** — keep it telegraphic
(number, link, one-line consequence). Detail lives in the files.

## Format

```md
---
status: accepted | superseded | rejected
date: YYYY-MM-DD
---

# NNNN — <title>

## Context
## Decision
## Consequences
```

## Index

| # | Decision | Status | Consequence |
|---|---|---|---|
| 0001 | [State sync: manifest-based file sync](0001-state-sync.md) | accepted | manifest+blob protocol, LWW per file; transport swappable to Iroh later |
| 0002 | [Snapshot format: single gzip+tar](0002-snapshot-format.md) | accepted | one-file snapshot; volumes via helper-container exec capture (not restic repo) |
| 0003 | [Dev IdP: Dex (not Zitadel)](0003-dev-identity-provider.md) | accepted | single-container dev OIDC; prod IdP is a separate decision |
| 0004 | [Zone-1 auth in the UI BFF, not the peer](0004-ui-bff-auth.md) | accepted | BFF holds OIDC session + proxies /api; peer stays plaintext on trusted net |
