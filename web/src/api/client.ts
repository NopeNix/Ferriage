import type { PeerInfo, StacksResponse } from "./types";

// The UI talks to the BFF at /api/*, which proxies to the peer.
// Phase 2.2 (OIDC) will add the session cookie here; fetch already
// sends same-origin cookies by default.

// Session state from GET /auth/me (BFF, Zone 1). The SPA never sees
// the session cookie (HttpOnly); it only needs the CSRF token so
// writes can present it.
export interface Session {
  sub: string;
  csrf: string;
}

let sessionPromise: Promise<Session | null> | null = null;

export function getSession(): Promise<Session | null> {
  if (!sessionPromise) {
    sessionPromise = fetch("/auth/me")
      .then(async (res) => (res.ok ? ((await res.json()) as Session) : null))
      .catch(() => null);
  }
  return sessionPromise;
}

export function resetSession() {
  sessionPromise = null;
}

async function get<T>(path: string): Promise<T> {
  const res = await fetch(`/api${path}`);
  if (!res.ok) throw new Error(`${path}: ${res.status} ${res.statusText}`);
  return (await res.json()) as T;
}

export const api = {
  stacks: () => get<StacksResponse>("/stacks"),
  peer: () => get<PeerInfo>("/peer"),
  session: getSession,
};
