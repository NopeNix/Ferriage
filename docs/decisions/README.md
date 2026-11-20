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
| 0005 | [Logs/stats/exec + Monaco (Phase 3 item 2)](0005-logs-stats-exec-monaco.md) | accepted | polling JSON for logs/stats; hand-rolled RFC 6455 console; Monaco vendored offline; BFF raw upgrade pipe for WS |
| 0006 | [SSH onboarding (Phase 3 item 3)](0006-ssh-onboarding.md) | accepted | native Go SSH, key-only, TO-FU/pinned host keys, get.docker.com install, peer pulls from operator peer |
| 0007 | [Shoutrrr notifications (Phase 3 item 4)](0007-shoutrrr-notifications.md) | accepted | env-only URLs (FERRIAGE_NOTIFY_URLS, option A), best-effort async, redacted GET /notify + test |
| 0008 | [Audit log (Phase 3 item 5)](0008-audit-log.md) | accepted | append-only JSONL in .ferriage/, middleware-recorded mutating actions, actor via X-Ferriage-Actor from BFF, GET /audit read-only |
| 0009 | [Auto-update (Phase 3 item 6)](0009-auto-update.md) | accepted | per-node digest-poll loop, poll/auto per stack, stateful→poll default, snapshot-before-update, Shoutrrr events |
| 0010 | [MCP server (Phase 3 item 7)](0010-mcp-server.md) | accepted | hand-rolled MCP Streamable HTTP at POST /mcp (stateless), tools dispatch in-process through the peer mux, BFF OIDC gate |
