// Zone-1 auth for the Ferriage UI BFF (docs/security.md): OIDC-only
// sessions, strict cookies, CSRF, rate limiting, no info leakage.
// Everything reachable without authentication is treated as hostile.
//
// Session shape (HMAC-signed cookie, no external store — UI is
// stateless, CONCEPT):
//   ferriage_session = <base64url(payload)>.<base64url(hmac)>
//   payload = { sub, exp, csrf, iat }
// The OIDC dance state (state/nonce/pkce) rides in a short-lived
// unsigned cookie `ferriage_oidc_state` (it's only ever compared
// against the provider's response; signing it is redundant).
import { createHmac, timingSafeEqual, randomBytes } from "node:crypto";
import * as client from "openid-client";

const SESSION_MAX_AGE = 8 * 60 * 60; // seconds

function b64url(buf) {
  return Buffer.from(buf).toString("base64url");
}

function hmac(key, data) {
  return createHmac("sha256", key).update(data).digest();
}

function parseCookies(header) {
  const out = {};
  for (const part of String(header || "").split(";")) {
    const i = part.indexOf("=");
    if (i > 0) out[part.slice(0, i).trim()] = part.slice(i + 1).trim();
  }
  return out;
}

function rateLimit(max, windowMs) {
  const hits = new Map();
  return (key) => {
    const now = Date.now();
    const arr = (hits.get(key) || []).filter((t) => now - t < windowMs);
    if (arr.length >= max) return false;
    arr.push(now);
    hits.set(key, arr);
    if (hits.size > 10000) hits.clear();
    return true;
  };
}

export class Auth {
  constructor({ issuer, clientId, clientSecret, redirectUri, sessionSecret, secure }) {
    if (!issuer || !clientId || !clientSecret || !redirectUri || !sessionSecret) {
      throw new Error("auth: issuer/clientId/clientSecret/redirectUri/sessionSecret required");
    }
    this.issuer = issuer;
    this.clientId = clientId;
    this.clientSecret = clientSecret;
    this.redirectUri = redirectUri;
    this.secret = sessionSecret;
    this.secure = Boolean(secure);
    this.config = null;
    this.allowLogin = rateLimit(10, 60_000);
    this.allowCallback = rateLimit(20, 60_000);
    this.allowApi = rateLimit(600, 60_000);
  }

  // Lazy discovery, cached. Failures must not leak IdP internals.
  // Plaintext HTTP is allowed ONLY for localhost issuers (dev IdP on
  // the home box); security.md mandates TLS everywhere in prod.
  async init() {
    if (!this.config) {
      const metadata = {
        response_types: ["code"],
        grant_types: ["authorization_code"],
        code_challenge_methods_supported: ["S256"],
      };
      const auth = client.ClientSecretPost(this.clientSecret);
      const options = this.issuer.startsWith("http://localhost")
        ? { execute: [client.allowInsecureRequests] }
        : undefined;
      this.config = await client.discovery(new URL(this.issuer), this.clientId, metadata, auth, options);
    }
    return this.config;
  }

  cookie(name, value, maxAge) {
    const parts = [
      `${name}=${value}`,
      "Path=/",
      "HttpOnly",
      "SameSite=Lax",
      `Max-Age=${maxAge}`,
    ];
    if (this.secure) parts.push("Secure");
    return parts.join("; ");
  }

  clearCookie(name) {
    return `${name}=; Path=/; HttpOnly; SameSite=Lax; Max-Age=0`;
  }

  sign(payload) {
    const body = b64url(JSON.stringify(payload));
    return `${body}.${b64url(hmac(this.secret, body))}`;
  }

  verify(token) {
    const [body, sig] = String(token || "").split(".");
    if (!body || !sig) return null;
    const expected = hmac(this.secret, body);
    let given;
    try {
      given = Buffer.from(sig, "base64url");
    } catch {
      return null;
    }
    if (given.length !== expected.length || !timingSafeEqual(given, expected)) return null;
    let payload;
    try {
      payload = JSON.parse(Buffer.from(body, "base64url").toString("utf8"));
    } catch {
      return null;
    }
    if (typeof payload.exp !== "number" || payload.exp < Date.now() / 1000) return null;
    if (!payload.sub || !payload.csrf) return null;
    return payload;
  }

  sessionFrom(req) {
    const cookies = parseCookies(req.headers.cookie);
    return this.verify(cookies.ferriage_session);
  }

  clientIp(req) {
    return String(req.headers["x-forwarded-for"] || req.socket.remoteAddress || "unknown").split(",")[0].trim();
  }

  // Returns true if the request may proceed.
  checkApi(req) {
    if (!this.allowApi(this.clientIp(req))) return false;
    const session = this.sessionFrom(req);
    if (!session) return false;
    if (["POST", "PUT", "PATCH", "DELETE"].includes(req.method)) {
      const csrf = req.headers["x-csrf-token"];
      if (typeof csrf !== "string" || csrf.length !== session.csrf.length) return false;
      try {
        if (!timingSafeEqual(Buffer.from(csrf), Buffer.from(session.csrf))) return false;
      } catch {
        return false;
      }
    }
    return true;
  }

  sessionPayload(sub) {
    const now = Math.floor(Date.now() / 1000);
    return {
      sub,
      exp: now + SESSION_MAX_AGE,
      iat: now,
      csrf: randomBytes(32).toString("hex"),
    };
  }

  async loginUrl() {
    const config = await this.init();
    const state = client.randomState();
    const nonce = client.randomNonce();
    const pkce = client.randomPKCECodeVerifier();
    const challenge = await client.calculatePKCECodeChallenge(pkce);
    const url = new URL(config.serverMetadata().authorization_endpoint);
    url.searchParams.set("client_id", this.clientId);
    url.searchParams.set("redirect_uri", this.redirectUri);
    url.searchParams.set("response_type", "code");
    url.searchParams.set("scope", "openid profile email");
    url.searchParams.set("state", state);
    url.searchParams.set("nonce", nonce);
    url.searchParams.set("code_challenge", challenge);
    url.searchParams.set("code_challenge_method", "S256");
    return { url: url.toString(), state, nonce, pkce };
  }

  async callback(req) {
    const config = await this.init();
    const cookies = parseCookies(req.headers.cookie);
    const st = cookies.ferriage_oidc_state;
    if (!st) throw new Error("missing oidc state");
    const { state, nonce, pkce } = JSON.parse(Buffer.from(st, "base64url").toString("utf8"));
    const currentUrl = new URL(req.url, `http://${req.headers.host}`);
    const tokens = await client.authorizationCodeGrant(config, currentUrl, {
      expectedState: state,
      expectedNonce: nonce,
      pkceCodeVerifier: pkce,
    });
    const claims = tokens.claims("id_token");
    if (!claims?.sub) throw new Error("no subject");
    return claims.sub;
  }
}
