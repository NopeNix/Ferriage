// Zone-1 auth unit tests: session signing, cookies, CSRF, rate limits.
// Uses node:test so no framework installs are needed.
import { test } from "node:test";
import assert from "node:assert/strict";
import { Auth } from "./auth.mjs";

function makeAuth() {
  return new Auth({
    issuer: "http://localhost:18081",
    clientId: "ferriage",
    clientSecret: "dev-secret",
    redirectUri: "http://localhost:19080/auth/callback",
    sessionSecret: "test-session-secret-for-unit-tests",
    secure: false,
  });
}

test("session round-trip: sign then verify", () => {
  const a = makeAuth();
  const token = a.sign(a.sessionPayload("user-123"));
  const payload = a.verify(token);
  assert.equal(payload.sub, "user-123");
  assert.ok(payload.csrf);
});

test("tampered session is rejected", () => {
  const a = makeAuth();
  const token = a.sign(a.sessionPayload("user-123"));
  const [body] = token.split(".");
  assert.equal(a.verify(`${body}.invalidsignature=`), null);
});

test("expired session is rejected", () => {
  const a = makeAuth();
  const payload = a.sessionPayload("user-123");
  payload.exp = Math.floor(Date.now() / 1000) - 10;
  assert.equal(a.verify(a.sign(payload)), null);
});

test("cookies: secure flag and clearing", () => {
  const a = makeAuth();
  const c = a.cookie("ferriage_session", "abc", 3600);
  assert.match(c, /^ferriage_session=abc;/);
  assert.match(c, /HttpOnly/);
  assert.match(c, /SameSite=Lax/);
  assert.doesNotMatch(c, /Secure/);
  const secure = new Auth({
    issuer: "x", clientId: "x", clientSecret: "x", redirectUri: "x", sessionSecret: "x", secure: true,
  });
  assert.match(secure.cookie("n", "v", 1), /Secure/);
  assert.match(a.clearCookie("ferriage_session"), /Max-Age=0/);
});

test("checkApi: unauthenticated GET denied, authenticated allowed", () => {
  const a = makeAuth();
  const denied = { method: "GET", headers: {}, socket: {} };
  assert.equal(a.checkApi(denied), false);
  const token = a.sign(a.sessionPayload("u"));
  const ok = { method: "GET", headers: { cookie: `ferriage_session=${token}` }, socket: {} };
  assert.equal(a.checkApi(ok), true);
});

test("checkApi: write without CSRF denied, with CSRF allowed", () => {
  const a = makeAuth();
  const payload = a.sessionPayload("u");
  const token = a.sign(payload);
  const noCsrf = { method: "POST", headers: { cookie: `ferriage_session=${token}` }, socket: {} };
  assert.equal(a.checkApi(noCsrf), false);
  const wrongCsrf = {
    method: "POST",
    headers: { cookie: `ferriage_session=${token}`, "x-csrf-token": "wrong" },
    socket: {},
  };
  assert.equal(a.checkApi(wrongCsrf), false);
  const ok = {
    method: "POST",
    headers: { cookie: `ferriage_session=${token}`, "x-csrf-token": payload.csrf },
    socket: {},
  };
  assert.equal(a.checkApi(ok), true);
});

test("rate limit: login bucket trips for a flooding IP", () => {
  const a = makeAuth();
  for (let i = 0; i < 10; i++) assert.equal(a.allowLogin("1.2.3.4"), true);
  assert.equal(a.allowLogin("1.2.3.4"), false, "11th login attempt must be denied");
  assert.equal(a.allowLogin("5.6.7.8"), true, "other IP unaffected");
});

test("constructor rejects missing config", () => {
  assert.throws(() => new Auth({}), /required/);
});
