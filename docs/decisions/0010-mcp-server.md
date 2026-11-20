---
status: accepted
date: 2026-11-20
---

# 0010 — MCP server (Phase 3 item 7)

## Context
CONCEPT should-have: "MCP server surface (the homelab is AI-heavy)."
Every peer operation already has a typed HTTP surface (`internal/api`)
that the UI and CLI share (API-first). An MCP server is a **protocol
translation** over that surface, not a new backend: AI agents must be
able to list stacks, deploy, read logs, run one-shot commands, and
check updates without a browser or a CLI.

Two transports were considered:

- **stdio**: a second binary (`ferriage-mcp`) that speaks MCP on
  stdin/stdout and shells the CLI or HTTP API. It would have no daemon
  access of its own, would need a second build/packaging story, and
  the AI client would have to run on each host — the opposite of the
  peer mesh (the peer IS the agent; the AI talks TO a peer).
- **Streamable HTTP on the peer** (`POST /mcp`): the peer process
  already owns the daemon, the state dir, and the audit log. One
  endpoint, one binary, and the existing BFF/OIDC gate can front it
  for remote AI clients exactly like it fronts the REST API.

The homelab case is "an AI agent driving a peer over the network,"
which is HTTP. stdio adds nothing for a peer that already serves.

## Decision
- **Transport: MCP Streamable HTTP, hand-rolled** (no SDK dependency).
  The MCP surface is JSON-RPC 2.0 over HTTP — same philosophy as the
  hand-rolled RFC 6455 console (ADR 0005): one small protocol, zero
  new dependencies, full control of the wire. The peer serves
  •
  `POST /mcp` (single-message JSON responses; SSE streaming is a
    future option — plain JSON is sufficient for the tool set),
  • `GET /mcp` → `MaxTools` listing via the same handler,
  • `DELETE /mcp` (session-less close) → 202.
  Session management: **stateless** — no `Mcp-Session-Id` is issued;
  every request is self-contained. Stateless is simpler, survives
  restarts, and matches the peer's "no special node" model. The
  protocol version negotiated is `2025-03-26`
  (the Streamable HTTP spec version current at decision time); the
  server also accepts older `MCP-Protocol-Version` headers and
  replies with the same value.
- **Dispatch: tool call = in-process request through the peer mux.**
  `internal/mcp` receives the bound mux (the same one `cmd/peer`
  builds). A `tools/call` maps the tool name to (HTTP method, peer
  path, JSON body) and replays it as an `httptest`-style request
  through the mux — so the tool sees **exactly** what the UI sees:
  same handlers, same validation, same error bodies, same audit
  middleware, same actor header. Headers: the caller's
  `X-Ferriage-Actor` is passed through (BFF-stamped for browser
  traffic; `mcp` as the default actor for direct peer traffic — an
  AI client IS an operator, Zone 2 trusted). This is the same
  "one typed contract" rule as the CLI twin (Phase 3 item 1):
  there is no second implementation of any operation.
- **Tool set (15 tools)**: a curated, safe subset — everything an
  agent needs to operate a homelab, minus the surfaces that are
  inherently interactive or secret-bearing:
  - `list_stacks`, `get_peer` (read)
  - `deploy_stack` (health-gated)
  - `export_stack` / `import_stack` (snapshot out/in; import takes
    base64 gzip in the tool payload — binary snapshots do not cross
    JSON-RPC as raw bytes)
  - `get_logs`, `get_stats`, `exec_in_service` (one-shot only — the
    interactive console is WebSocket, not a tool)
  - `get_compose`, `save_compose`
  - `check_update`, `set_update_policy`, `run_update`
  - `get_audit` (observability for agents)
  - `onboard_host` (address + key — secret-bearing by design,
    Zone 2 trusted; the key is never echoed back)

  Excluded: `WS console` (interactive, not a tool), `/sync/*`
  (mesh wire — agents must not speak it), `/notify` (env-only
  secrets — no tool; notifications are observed via `get_audit` /
  update reports), `/openapi.json` (the contract is not an op).
  Read-only vs mutating is marked in `tools/list` annotations
  (`readOnly` / `destructive`) so AI clients can gate destructive
  calls behind confirmation.

- **Errors**: peer errors (HTTP 4xx/5xx with `{"error": ...}`) become
  MCP tool errors (`isError: true`, message = the peer's error
  string) — never a JSON-RPC protocol error. Unknown tools and
  malformed arguments are JSON-RPC `-32602` (invalid params).
  Unknown methods are `-32601`. Protocol handshakes complete with
  `initialize` → `2025-03-26`.

- **Surfaces**:
  - `POST /mcp` (peer) + `GET /mcp` + `DELETE /mcp` (peer) —
    registered via `mcp.Register(mux, peerMux)`; the BFF proxies
    `/mcp` behind the same OIDC session gate as `/api/*` (ADR 0004:
    the peer itself stays plaintext on the trusted net).
  - CLI: `ferriage mcp [call <tool> --args '<json>']` — a thin
    JSON-RPC client twin (proves the surface from the operator's
    terminal, like every other twin).
  - UI: no new card (the surface is for AI clients; it is observable
    via `ferriage mcp tools` and the audit log). A `MCPStatus`
    read-only card is deferred — add it only if an operator asks.

- **Audit**: MCP tool calls that mutate state flow through the peer
  mux and are recorded by `internal/audit` like any other call
  (actor = `mcp` or the BFF-stamped actor). The `POST /mcp`
  envelope itself is never audited (it is mesh machinery, like
  `/sync/*`); the inner tool call is, so an MCP `deploy_stack`
  lands as `deploy`/`mcp-demo`, not as a bare `/mcp` line —
  dispatch records it with the same classifier the middleware
  uses (`audit.Record`). Read tools are GETs and are not
  audited. No new audit actions.

## Consequences
- `internal/mcp` is protocol-only: it knows the tool table and the
  JSON-RPC envelope, never Docker, Compose, or the state dir.
- The tool table is the contract: `tools/list` is generated from
  the same table `tools/call` dispatches on, so the two can never
  drift. Adding a tool = one table row.
- `POST /mcp` is pre-login-reachable **only through the BFF**
  (OIDC gate). Direct peer access is trusted-net only (ADR 0004).
- SSE streaming and stdio remain future options if an AI client
  needs them; neither changes the tool table.
