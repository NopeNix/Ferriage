import type { DeployResult, Error as ApiError, Peer, SnapshotManifest, Stacks } from "./types";

// The UI talks to the BFF at /api/*, which proxies to the peer.
// The peer API is typed by the OpenAPI contract served at
// /openapi.json (internal/api); types.ts is generated from it.
// Writes carry the session CSRF token from GET /auth/me (ADR 0004).

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

async function request<T>(method: string, path: string): Promise<T> {
  const init: RequestInit = { method };
  if (method !== "GET" && method !== "HEAD") {
    const session = await getSession();
    if (!session) throw new Error(`${path}: 401 Unauthorized`);
    init.headers = { "x-csrf-token": session.csrf };
  }
  const res = await fetch(`/api${path}`, init);
  if (!res.ok) {
    const body = (await res.json().catch(() => null)) as ApiError | null;
    throw new Error(`${path}: ${res.status} ${body?.error ?? res.statusText}`);
  }
  return (await res.json()) as T;
}

const enc = (name: string) => encodeURIComponent(name);

export const api = {
  stacks: () => request<Stacks>("GET", "/stacks"),
  peer: () => request<Peer>("GET", "/peer"),
  session: getSession,
  deploy: (name: string) => request<DeployResult>("POST", `/stacks/${enc(name)}/deploy`),
  exportStack: async (name: string): Promise<Blob> => {
    const session = await getSession();
    if (!session) throw new Error(`/stacks/${name}/export: 401 Unauthorized`);
    const res = await fetch(`/api/stacks/${enc(name)}/export`, {
      method: "POST",
      headers: { "x-csrf-token": session.csrf },
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as ApiError | null;
      throw new Error(`/stacks/${name}/export: ${res.status} ${body?.error ?? res.statusText}`);
    }
    return await res.blob();
  },
  importStack: async (name: string, archive: Blob): Promise<SnapshotManifest> => {
    const session = await getSession();
    if (!session) throw new Error(`/stacks/${name}/import: 401 Unauthorized`);
    const res = await fetch(`/api/stacks/${enc(name)}/import`, {
      method: "POST",
      headers: { "x-csrf-token": session.csrf, "content-type": "application/gzip" },
      body: archive,
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as ApiError | null;
      throw new Error(`/stacks/${name}/import: ${res.status} ${body?.error ?? res.statusText}`);
    }
    return (await res.json()) as SnapshotManifest;
  },
};
