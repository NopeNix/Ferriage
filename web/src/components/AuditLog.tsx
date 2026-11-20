import { useCallback, useEffect, useState } from "react";
import { History, Loader2, RefreshCw } from "lucide-react";
import { api } from "../api/client";
import type { AuditEntry } from "../api/types";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

// AuditLog is the read-only audit card (ADR 0008): who did
// what, when. Entries come from the peer's append-only JSONL
// (one line per mutating operator action, actor reported by
// the BFF from the verified session). There is no write or
// delete UI — the log is append-only by API design.
export function AuditLog() {
  const [entries, setEntries] = useState<AuditEntry[] | null>(null);
  const [actor, setActor] = useState("");
  const [action, setAction] = useState("");
  const [loading, setLoading] = useState(true);

  const load = useCallback(async () => {
    setLoading(true);
    try {
      const resp = await api.audit({
        limit: 50,
        actor: actor.trim() || undefined,
        action: action.trim() || undefined,
      });
      setEntries(resp.entries);
    } catch {
      setEntries(null);
    } finally {
      setLoading(false);
    }
  }, [actor, action]);

  useEffect(() => {
    void load();
  }, [load]);

  return (
    <Card data-testid="audit-log">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <History className="size-5" />
          Audit log
        </CardTitle>
        <CardDescription>
          Who did what, when — recorded by the peer for every mutating action (deploy, export, import, compose save, exec, onboard).
          Read-only; the peer appends, nobody edits.
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        <div className="flex flex-wrap items-end gap-2">
          <label className="grid gap-1 text-xs text-muted-foreground">
            Actor
            <input
              className="rounded-md border border-input bg-background px-2 py-1 text-sm text-foreground"
              placeholder="any"
              value={actor}
              onChange={(e) => setActor(e.target.value)}
              data-testid="audit-actor-filter"
            />
          </label>
          <label className="grid gap-1 text-xs text-muted-foreground">
            Action
            <input
              className="rounded-md border border-input bg-background px-2 py-1 text-sm text-foreground"
              placeholder="deploy, exec, …"
              value={action}
              onChange={(e) => setAction(e.target.value)}
              data-testid="audit-action-filter"
            />
          </label>
          <Button variant="outline" size="sm" onClick={() => void load()} data-testid="audit-refresh">
            {loading ? <Loader2 className="size-4 animate-spin" /> : <RefreshCw className="size-4" />}
            Refresh
          </Button>
        </div>
        {entries === null ? (
          <p data-testid="audit-error" className="text-sm text-muted-foreground">
            Could not load the audit log.
          </p>
        ) : entries.length === 0 ? (
          <p data-testid="audit-empty" className="text-sm text-muted-foreground">
            No audit entries yet — deploy a stack or run a command and it will be recorded here.
          </p>
        ) : (
          <div className="grid gap-1">
            {entries.map((e, i) => (
              <div key={`${e.ts}-${i}`} data-testid="audit-entry" className="flex flex-wrap items-baseline gap-x-3 gap-y-0.5 text-sm">
                <span className="font-mono text-xs text-muted-foreground">{e.ts}</span>
                <span className="font-medium">{e.actor}</span>
                <span className="rounded bg-muted px-1.5 py-0.5 text-xs">{e.action}</span>
                {e.target && <span className="text-muted-foreground">{e.target}</span>}
                <span className={e.status >= 400 ? "text-destructive" : "text-emerald-600"}>{e.status}</span>
                {e.detail && <span className="text-xs text-muted-foreground">{e.detail}</span>}
              </div>
            ))}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
