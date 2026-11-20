import type { ComposeFile, DeployResult, Error as ApiError, ExecResult, Logs, NotifyStatus, NotifyTest, OnboardResponse, Peer, SnapshotManifest, Stacks, Stats } from "./types";

// The UI talks to the BFF at /api/*, which proxies to the peer.
// The peer API is typed by the OpenAPI contract served at
// /openapi.json (internal/api); types.ts is generated from it.
// Writes carry the session CSRF token from GET /auth/me (ADR 0004).

export interface Session {
  sub: string;
  csrf: string;
}

export function consoleUrl(stack: string, svc: string): string {
  const proto = window.location.protocol === "https:" ? "wss:" : "ws:";
  return `${proto}//${window.location.host}/api/stacks/${encodeURIComponent(stack)}/services/${encodeURIComponent(svc)}/console`;
}

let sessionPromise: Promise<Session | null> | null = null;

export function getSession(): Promise<Session | null> {
  if (!sessionPromise) {
    sessionPromise = fetch("/auth/me")
      .then(async (res) => {
        if (res.ok) return (await res.json()) as Session;
        // Dev mode: the BFF has no OIDC issuer configured and
        // proxies /api/* without auth (ADR 0004). The /auth/me
        // route is 404 there; treat it as an open dev session
        // with a stub CSRF (the peer does not check CSRF in dev).
        if (res.status === 404) return { sub: "dev", csrf: "dev" };
        return null;
      })
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
  logs: (name: string, svc: string, tail = 200) =>
    request<Logs>("GET", `/stacks/${enc(name)}/services/${enc(svc)}/logs?tail=${tail}`),
  stats: (name: string) => request<Stats>("GET", `/stacks/${enc(name)}/stats`),
  exec: async (name: string, svc: string, cmd: string[]): Promise<ExecResult> => {
    const session = await getSession();
    if (!session) throw new Error("401 Unauthorized");
    const res = await fetch(`/api/stacks/${enc(name)}/services/${enc(svc)}/exec`, {
      method: "POST",
      headers: { "x-csrf-token": session.csrf, "content-type": "application/json" },
      body: JSON.stringify({ cmd }),
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as ApiError | null;
      throw new Error(`${res.status} ${body?.error ?? res.statusText}`);
    }
    return (await res.json()) as ExecResult;
  },
  compose: (name: string) => request<ComposeFile>("GET", `/stacks/${enc(name)}/compose`),
  saveCompose: async (name: string, content: string): Promise<ComposeFile> => {
    const session = await getSession();
    if (!session) throw new Error("401 Unauthorized");
    const res = await fetch(`/api/stacks/${enc(name)}/compose`, {
      method: "PUT",
      headers: { "x-csrf-token": session.csrf, "content-type": "application/json" },
      body: JSON.stringify({ content }),
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as ApiError | null;
      throw new Error(`${res.status} ${body?.error ?? res.statusText}`);
    }
    return (await res.json()) as ComposeFile;
  },
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
  // Zero-touch SSH onboarding (ADR 0006): address + key, Docker
  // check, one Ferriage container. The key is read client-side
  // and sent once; the peer keeps it in memory only.
  onboardHost: async (req: {
    address: string;
    user?: string;
    privateKey: string;
    image?: string;
    peerUrl?: string;
    port?: number;
  }): Promise<OnboardResponse> => {
    const session = await getSession();
    if (!session) throw new Error(`/hosts/onboard: 401 Unauthorized`);
    const res = await fetch(`/api/hosts/onboard`, {
      method: "POST",
      headers: { "x-csrf-token": session.csrf, "content-type": "application/json" },
      body: JSON.stringify({
        address: req.address,
        user: req.user || undefined,
        private_key: req.privateKey,
        image: req.image || undefined,
        peer_url: req.peerUrl || undefined,
        port: req.port || undefined,
      }),
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as (ApiError & Partial<OnboardResponse>) | null;
      // On partial failure the peer returns 502 with the steps.
      if (body && Array.isArray((body as OnboardResponse).steps)) {
        const partial = body as OnboardResponse;
        partial.error = partial.error || body.error || `HTTP ${res.status}`;
        return partial;
      }
      throw new Error(`/hosts/onboard: ${res.status} ${body?.error ?? res.statusText}`);
    }
    return (await res.json()) as OnboardResponse;
  },
  // Shoutrrr notifications (ADR 0007): redacted status + test.
  // URLs are env-only on the peer and never cross the API.
  notifyStatus: () => request<NotifyStatus>("GET", "/notify"),
  notifyTest: async (): Promise<NotifyTest> => {
    const session = await getSession();
    if (!session) throw new Error(`/notify/test: 401 Unauthorized`);
    const res = await fetch(`/api/notify/test`, {
      method: "POST",
      headers: { "x-csrf-token": session.csrf },
    });
    if (!res.ok) {
      const body = (await res.json().catch(() => null)) as ApiError | null;
      throw new Error(`/notify/test: ${res.status} ${body?.error ?? res.statusText}`);
    }
    return (await res.json()) as NotifyTest;
  },
};
