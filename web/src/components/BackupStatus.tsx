import { useCallback, useEffect, useState } from "react";
import { DatabaseBackup, Loader2, RefreshCw } from "lucide-react";
import { api } from "../api/client";
import type { BackupList } from "../api/types";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

// BackupStatus is the restic backup card (ADR 0011): repo status,
// snapshot history (newest first), and the three operator actions
// — back up now (one stack or the whole node), restore (with a
// health-gated deploy), and retention pruning. The repo password
// is env/file-only on the peer and is never shown: only whether
// one is configured.
export function BackupStatus() {
  const [data, setData] = useState<BackupList | null>(null);
  const [err, setErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [msg, setMsg] = useState<string | null>(null);

  const load = useCallback(async () => {
    try {
      const resp = await api.backup();
      setData(resp);
      setErr(null);
    } catch (e) {
      setErr(e instanceof Error ? e.message : String(e));
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function act(fn: () => Promise<{ detail?: string; healthy?: boolean }>, ok: string) {
    setBusy(true);
    setMsg(null);
    try {
      const r = await fn();
      setMsg(r.healthy !== undefined ? `${ok} (healthy=${r.healthy})` : ok);
      await load();
    } catch (e) {
      setMsg(`Failed: ${e instanceof Error ? e.message : String(e)}`);
    } finally {
      setBusy(false);
    }
  }

  return (
    <Card data-testid="backup-status">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <DatabaseBackup className="size-5" />
          Backups (restic)
        </CardTitle>
        <CardDescription>
          Incremental, deduplicated snapshots of stacks + volumes (ADR 0011). The repo password lives in the peer's env/file only — never here.
          {data?.status && !data.status.initialized && " Repo not initialized yet — run a backup to create it."}
          {data?.status && data.status.initialized && !data.status.password_set && " No password configured on the peer — backups are disabled."}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {err && (
          <p data-testid="backup-error" className="text-sm text-muted-foreground">
            Could not load backup status.
          </p>
        )}
        {data?.status && !err && (
          <>
            <div className="flex flex-wrap items-baseline gap-x-3 text-sm">
              <span className="font-mono text-xs text-muted-foreground">{data.status.repo}</span>
              <span className="text-muted-foreground">{data.status.snapshots} snapshot{data.status.snapshots === 1 ? "" : "s"}</span>
              {data.status.last_backup_at && (
                <span className="font-mono text-xs text-muted-foreground">last: {data.status.last_backup_at} ok={String(data.status.last_backup_ok)}</span>
              )}
            </div>
            {data.snapshots.length > 0 && (
              <div className="grid gap-1">
                {data.snapshots.map((s) => (
                  <div key={s.id} data-testid="backup-snapshot" className="flex flex-wrap items-baseline gap-x-3 text-sm">
                    <span className="font-mono text-xs">{s.id.slice(0, 12)}</span>
                    <span className="font-mono text-xs text-muted-foreground">{s.time}</span>
                    {s.tags && s.tags.length > 0 && <span className="text-xs text-muted-foreground">{s.tags.join(", ")}</span>}
                    <span className="text-xs text-muted-foreground">{s.size} bytes</span>
                  </div>
                ))}
              </div>
            )}
          </>
        )}
        {msg && (
          <p data-testid="backup-msg" className="text-sm text-muted-foreground">
            {msg}
          </p>
        )}
        <div className="flex flex-wrap gap-2">
          <Button variant="outline" size="sm" disabled={busy} onClick={() => void act(() => api.runBackup(""), "Node backup complete")} data-testid="backup-run-node">
            {busy ? <Loader2 className="size-4 animate-spin" /> : null} Back up node
          </Button>
          <Button variant="outline" size="sm" disabled={busy} onClick={() => void act(() => api.forgetBackups(), "Retention pruning done")} data-testid="backup-forget">
            Prune (keep last)
          </Button>
          <Button variant="outline" size="sm" onClick={() => void load()} data-testid="backup-refresh">
            <RefreshCw className="size-4" /> Refresh
          </Button>
        </div>
      </CardContent>
    </Card>
  );
}
