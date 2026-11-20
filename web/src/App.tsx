import { useCallback, useEffect, useState } from "react";
import { AlertTriangle, Ship, RefreshCw } from "lucide-react";
import { api } from "./api/client";
import type { PeerInfo, Stack } from "./api/types";
import { StackCard } from "./components/StackCard";
import { Badge } from "./components/ui/badge";
import { Button } from "./components/ui/button";

type State =
  | { status: "loading" }
  | { status: "error"; message: string }
  | { status: "ok"; stacks: Stack[]; peer: PeerInfo; sub: string | null };

export default function App() {
  const [state, setState] = useState<State>({ status: "loading" });

  const load = useCallback(async () => {
    setState({ status: "loading" });
    try {
      const [stacks, peer, session] = await Promise.all([api.stacks(), api.peer(), api.session()]);
      setState({ status: "ok", stacks: stacks.stacks, peer, sub: session?.sub ?? null });
    } catch (err) {
      setState({ status: "error", message: err instanceof Error ? err.message : String(err) });
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  const needsLogin = state.status === "error" && state.message.includes("401");

  return (
    <div className="min-h-dvh bg-background text-foreground">
      <header className="border-b border-border">
        <div className="mx-auto flex max-w-6xl items-center justify-between px-6 py-4">
          <h1 className="flex items-center gap-2 text-xl font-semibold tracking-tight">
            <Ship className="size-6" />
            Ferriage
          </h1>
          <div className="flex items-center gap-3">
            {state.status === "ok" && state.sub && (
              <span className="text-sm text-muted-foreground" data-testid="user-badge">
                {state.sub}
              </span>
            )}
            {state.status === "ok" && (
              <Badge variant="secondary" data-testid="peer-badge">
                peer {state.peer.peer_id.slice(0, 12)}… · {state.peer.version}
              </Badge>
            )}
            <Button variant="outline" size="sm" onClick={() => void load()}>
              <RefreshCw className="size-4" />
              Refresh
            </Button>
          </div>
        </div>
      </header>

      <main className="mx-auto max-w-6xl px-6 py-8">
        {state.status === "loading" && <p data-testid="loading">Loading stacks…</p>}

        {state.status === "error" && (
          <div data-testid="error" className="flex items-start gap-3 rounded-lg border border-destructive/50 p-4">
            <AlertTriangle className="mt-0.5 size-5 text-destructive" />
            <div>
              <p className="font-medium">
                {needsLogin ? "You are not logged in" : "Could not reach the peer"}
              </p>
              <p className="text-sm text-muted-foreground">
                {needsLogin
                  ? "Log in to view your stacks."
                  : `${state.message} — the peer may be down. A dead host degrades to “offline in UI”, never “management dead” (CONCEPT §9).`}
              </p>
              {needsLogin && (
                <Button className="mt-3" onClick={() => (window.location.href = "/auth/login")}>
                  Log in
                </Button>
              )}
            </div>
          </div>
        )}

        {state.status === "ok" && (
          <>
            <p className="mb-4 text-sm text-muted-foreground" data-testid="stack-count">
              {state.stacks.length} stack{state.stacks.length === 1 ? "" : "s"} adopted
            </p>
            {state.stacks.length === 0 ? (
              <p data-testid="empty">No stacks yet. Deploy one with the peer API or `docker compose up`.</p>
            ) : (
              <div className="grid gap-4 md:grid-cols-2">
                {state.stacks.map((s) => (
                  <StackCard key={s.name} stack={s} />
                ))}
              </div>
            )}
          </>
        )}
      </main>
    </div>
  );
}
