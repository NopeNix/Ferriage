// Ferriage UI backend-for-frontend (BFF): serves the built SPA and
// proxies /api/* to the peer. Zone-1 gate (docs/security.md): every
// /api/* request requires a valid OIDC session (see auth.mjs);
// /auth/* is the login dance. Phase 2.2. The frontend stays a
// stateless reader (CONCEPT: UI is a mesh participant).
import { createServer } from "node:http";
import net from "node:net";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";
import { Auth } from "./auth.mjs";

const PORT = Number(process.env.PORT || "5173");
const PEER = (process.env.FERRIAGE_PEER || "http://localhost:8080").replace(/\/$/, "");
const PEER_URL = new URL(PEER);
const DIST = path.join(path.dirname(fileURLToPath(import.meta.url)), "..", "dist");

const AUTH_ENABLED = Boolean(process.env.FERRIAGE_OIDC_ISSUER);
const auth = AUTH_ENABLED
  ? new Auth({
      issuer: process.env.FERRIAGE_OIDC_ISSUER,
      clientId: process.env.FERRIAGE_OIDC_CLIENT_ID,
      clientSecret: process.env.FERRIAGE_OIDC_CLIENT_SECRET,
      redirectUri: process.env.FERRIAGE_OIDC_REDIRECT_URI,
      sessionSecret: process.env.FERRIAGE_SESSION_SECRET,
      secure: process.env.FERRIAGE_COOKIE_SECURE === "1",
    })
  : null;

const TYPES = {
  ".html": "text/html; charset=utf-8",
  ".js": "text/javascript; charset=utf-8",
  ".css": "text/css; charset=utf-8",
  ".svg": "image/svg+xml",
  ".ico": "image/x-icon",
  ".json": "application/json",
  ".woff2": "font/woff2",
  ".woff": "font/woff",
};

function send(res, status, body) {
  res.writeHead(status, { "Content-Type": "application/json" });
  res.end(JSON.stringify(body));
}

function isWsUpgrade(req) {
  return (req.headers.upgrade || "").toLowerCase() === "websocket";
}

// WebSocket upgrades (the console) need a raw TCP pipe — fetch()
// cannot hijack. Forwards the request head verbatim to the
// peer and pipes bytes both ways until either side closes.
function proxyWsUpgrade(head, socket) {
  const upstream = net.createConnection(PEER_URL.port, PEER_URL.hostname, () => {
    upstream.write(head);
    socket.pipe(upstream);
    upstream.pipe(socket);
  });
  for (const sock of [socket, upstream]) {
    sock.on("error", () => {
      socket.destroy();
      upstream.destroy();
    });
  }
  socket.on("close", () => upstream.destroy());
  upstream.on("close", () => socket.destroy());
}

async function proxyApi(req, res) {
  const url = PEER + req.url.replace(/^\/api/, "");
  let upstream;
  try {
    upstream = await fetch(url, {
      method: req.method,
      headers: Object.fromEntries(
        Object.entries(req.headers).filter(([k]) => !["host", "connection", "cookie"].includes(k)),
      ),
      body: req.method === "GET" || req.method === "HEAD" ? undefined : req,
      duplex: "half",
    });
  } catch {
    send(res, 502, { error: "peer unreachable" });
    return;
  }
  const headers = {};
  for (const [k, v] of upstream.headers) {
    if (!["transfer-encoding", "connection"].includes(k)) headers[k] = v;
  }
  res.writeHead(upstream.status, headers);
  if (upstream.body) {
    for await (const chunk of upstream.body) res.write(chunk);
  }
  res.end();
}

async function serveStatic(req, res) {
  const urlPath = req.url.split("?")[0];
  let file = path.join(DIST, urlPath === "/" ? "index.html" : urlPath);
  if (!file.startsWith(DIST) || !path.extname(file)) file = path.join(DIST, "index.html");
  try {
    const data = await readFile(file);
    res.writeHead(200, { "Content-Type": TYPES[path.extname(file)] || "application/octet-stream" });
    res.end(data);
  } catch {
    send(res, 404, { error: "not found" });
  }
}

async function handleAuth(req, res) {
  const url = new URL(req.url, "http://bff");
  const ip = auth.clientIp(req);
  if (url.pathname === "/auth/login") {
    if (!auth.allowLogin(ip)) return send(res, 429, { error: "too many requests" });
    try {
      const { url: loginUrl, state, nonce, pkce } = await auth.loginUrl();
      const cookieState = Buffer.from(JSON.stringify({ state, nonce, pkce })).toString("base64url");
      res.writeHead(302, {
        Location: loginUrl,
        "Set-Cookie": auth.cookie("ferriage_oidc_state", cookieState, 600),
      });
      res.end();
    } catch {
      send(res, 502, { error: "authentication unavailable" });
    }
  } else if (url.pathname === "/auth/callback") {
    if (!auth.allowCallback(ip)) return send(res, 429, { error: "too many requests" });
    try {
      const sub = await auth.callback(req);
      const payload = auth.sessionPayload(sub);
      const session = auth.sign(payload);
      res.writeHead(302, {
        Location: "/",
        "Set-Cookie": [
          auth.cookie("ferriage_session", session, 8 * 60 * 60),
          auth.clearCookie("ferriage_oidc_state"),
        ],
      });
      res.end();
    } catch {
      send(res, 400, { error: "authentication failed" });
    }
  } else if (url.pathname === "/auth/me") {
    const session = auth.sessionFrom(req);
    if (!session) return send(res, 401, { error: "unauthorized" });
    send(res, 200, { sub: session.sub, csrf: session.csrf });
  } else if (url.pathname === "/auth/logout") {
    res.writeHead(302, {
      Location: "/",
      "Set-Cookie": [auth.clearCookie("ferriage_session"), auth.clearCookie("ferriage_oidc_state")],
    });
    res.end();
  } else {
    send(res, 404, { error: "not found" });
  }
}

const server = createServer((req, res) => {
  const url = new URL(req.url, "http://bff");
  if (url.pathname.startsWith("/auth/")) {
    if (!auth) return send(res, 404, { error: "not found" });
    handleAuth(req, res).catch(() => send(res, 500, { error: "internal error" }));
  } else if (url.pathname.startsWith("/api/")) {
    if (auth && !auth.checkApi(req)) return send(res, 401, { error: "unauthorized" });
    proxyApi(req, res).catch(() => send(res, 502, { error: "peer unreachable" }));
  } else {
    serveStatic(req, res).catch(() => send(res, 404, { error: "not found" }));
  }
});

server.on("upgrade", (req, socket, head) => {
  const url = new URL(req.url, "http://bff");
  if (!url.pathname.startsWith("/api/")) {
    socket.destroy();
    return;
  }
  if (auth && !auth.checkApi(req)) {
    socket.write("HTTP/1.1 401 Unauthorized\r\nConnection: close\r\n\r\n");
    socket.destroy();
    return;
  }
  // Rewrite the path: drop the /api prefix the SPA uses.
  const peerPath = req.url.replace(/^\/api/, "");
  const lines = [`${req.method} ${peerPath} HTTP/1.1`, ...Object.entries(req.headers).map(([k, v]) => `${k}: ${v}`), "", ""];
  proxyWsUpgrade(lines.join("\r\n"), socket);
});

server.listen(PORT, () =>
  console.log(`ferriage-web listening on :${PORT}, peer ${PEER}, auth=${AUTH_ENABLED ? "on" : "off (dev)"}`),
);
