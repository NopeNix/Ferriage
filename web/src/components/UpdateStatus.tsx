import { useCallback, useEffect, useState } from "react";
import { Loader2, RefreshCw, RefreshCcw } from "lucide-react";
import { api } from "../api/client";
import type { UpdateStatus as UpdateStatusT } from "../api/types";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

// UpdateStatus is the read-only auto-update card (ADR 0009):
// per-stack digest-poll reports. Policy is poll (report only)
// vs auto (snapshot + redeploy); stateful stacks (named volumes)
// default to poll even when the global default is auto. The
// policy write happens here as well — it is a trusted-operator
// action (Zone 2), audited by the peer as update.policy.
export function UpdateStatus() {
  const [stacks, setStacks] = useState<UpdateStatusT[] | null>(null);
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await api.update();
      setStacks(resp.stacks);
    } catch {
      setStacks(null);
    } finally {
      setLoading(false);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function setPolicy(name: string, policy: "poll" | "auto") {
    try {
      await api.setUpdatePolicy(name, policy);
      await load();
    } catch {
      // The banner above will show the stale state; a refresh
      // re-reads. Keep it simple: the peer is the source of truth.
      await load();
    }
  }

  return (
    <Card data-testid="update-status">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <RefreshCcw className="size-5" />
          Auto-update
        </CardTitle>
        <CardDescription>
          Per-stack digest poll. <code>poll</code> reports only; <code>auto</code> snapshots then redeploys.
          Stateful stacks (named volumes) default to <code>poll</code> — opt in explicitly.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {stacks === null ? (
          <p data-testid="update-error" className="text-sm text-muted-foreground">
            Could not load update status.
          </p>
        ) : stacks.length === 0 ? (
          <p data-testid="update-empty" className="text-sm text-muted-foreground">
            No stacks yet — update reports appear once stacks exist.
          </p>
        ) : (
          stacks.map((s) => (
            <div key={s.stack} data-testid="update-stack" className="grid gap-1 text-sm">
              <div className="flex flex-wrap items-baseline gap-x-3">
                <span className="font-medium">{s.stack}</span>
                <span className="rounded bg-muted px-1.5 py-0.5 text-xs">{s.policy}</span>
                <span className="text-muted-foreground">state: {s.state}</span>
                {s.stateful && <span className="text-muted-foreground">stateful (named volume)</span>}
                {s.checked_at && <span className="font-mono text-xs text-muted-foreground">{s.checked_at}</span>}
                <Button variant="outline" size="sm" onClick={() => void setPolicy(s.stack, s.policy === "auto" ? "poll" : "auto")} data-testid={`update-policy-${s.stack}`}>
                  {s.policy === "auto" ? "Set poll" : "Set auto"}
                </Button>
              </div>
              {s.services.map((svc) => (
                <p key={svc.service} className="text-xs text-muted-foreground">
                  {svc.service} {svc.image} — {svc.outcome}
                  {svc.detail ? ` (${svc.detail})` : ""}
                </p>
              ))}
              {s.last_run && (
                <p className="text-xs text-muted-foreground">
                  last run: {s.last_run.ok ? "ok" : "failed"} at {s.last_run.at} (snapshot {s.last_run.snapshot})
                </p>
              )}
              {s.error && <p className="text-xs text-destructive">{s.error}</p>}
            </div>
          ))
        )}
        <div>
          <Button variant="outline" size="sm" onClick={() => void load()} data-testid="update-refresh">
            {loading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            Refresh
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
