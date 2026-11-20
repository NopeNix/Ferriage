---
date: 2026-11-20
session: 013
slug: phase3-logs-stats-exec
topics: [phase3, logs, stats, exec, monaco, websocket, bff, web]
---

# Session 013 learnings — Phase 3 (2/8)

- **docker `errdefs.IsNotFound` is staticcheck-deprecated
  (SA1019).** Use `github.com/containerd/errdefs` (already an
  indirect dep in go.mod). golangci-lint catches it in layer 1.
- **`@monaco-editor/loader` pins monaco via jsDelivr CDN and
  you cannot change the pinned version** — only the base URL.
  Override with `loader.config({paths:{vs:"/monaco/vs"}})` and
  vendor the local `monaco-editor` package's `min/` into
  `dist/monaco` via a `postbuild` script. Kills CDN dependency
  and version skew in one move; ~25MB in dist (its own cost).
- **BFF `fetch()` proxying 502s on WebSocket upgrades.** Node's
  `server.on("upgrade")` + raw `net.createConnection` pipe is
  the fix; rewrite `/api` prefix, forward the request head
  verbatim, pipe both ways. Zone-1 session check stays in the
  upgrade handler.
- **Dev-mode `/auth/me` is 404 when OIDC is off** (BFF route
  only exists with auth). Browser writes then 401 client-side.
  Treat 404 as an open dev session in `getSession()` — same
  contract as the BFF's dev proxy.
- **Playwright python via `uvx --with playwright`** works but
  wants its own chromium build; pass
  `executable_path` to an existing
  `~/.cache/ms-playwright/chromium-*/chrome-linux64/chrome`.
- **Docker stats CPU%** = (cpuTotal-preCpuTotal) /
  (sysTotal-preSysTotal) × onlineCPUs × 100; guard sysDelta 0.
  `StatsResponseReader.Body` is a field, not a method.
- **docker exec attach stdin is raw, stdout framed** (from
  session 007) — for one-shot exec, demux with stdcopy;
  for the TTY console, pass bytes through.
- `file_editor` `str_replace` needs exact tabs; the preview
  renders tabs as spaces, so mismatches look identical.
  Use `sed -n 'Np' | cat -A` to see real bytes, or
  `python3` with `\t` anchors.
