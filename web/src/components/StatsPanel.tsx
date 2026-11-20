import { useEffect, useState } from "react";
import { api } from "../api/client";
import type { ContainerStats } from "../api/types";
import { Badge } from "./ui/badge";

const fmtBytes = (n: number): string => {
  if (n < 1024) return `${n} B`;
  if (n < 1024 * 1024) return `${(n / 1024).toFixed(1)} KiB`;
  return `${(n / (1024 * 1024)).toFixed(1)} MiB`;
};

export function StatsPanel({ stack }: { stack: string }) {
  const [stats, setStats] = useState<ContainerStats[]>([]);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;
    const load = async () => {
      try {
        const res = await api.stats(stack);
        if (!cancelled) {
          setStats(Array.isArray(res.stats) ? res.stats : []);
          setError(null);
        }
      } catch (err) {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      }
    };
    void load();
    const id = setInterval(() => void load(), 3000);
    return () => {
      cancelled = true;
      clearInterval(id);
    };
  }, [stack]);

  if (error && stats.length === 0) {
    return (
      <p data-testid="stats-error" className="text-sm text-destructive">
        stats: {error}
      </p>
    );
  }
  if (stats.length === 0) {
    return <p data-testid="stats-empty" className="text-sm text-muted-foreground">Collecting stats…</p>;
  }
  return (
    <ul className="grid gap-2 sm:grid-cols-2" data-testid="stats-panel">
      {stats.map((s) => (
        <li key={s.container} className="rounded-md border p-3 text-sm">
          <p className="flex items-center justify-between gap-2">
            <span className="font-medium">{s.container}</span>
            <Badge variant={s.cpu_percent > 80 ? "destructive" : "secondary"}>
              CPU {s.cpu_percent.toFixed(1)}%
            </Badge>
          </p>
          <p className="mt-1 text-muted-foreground">
            mem {fmtBytes(s.memory_bytes)} / {fmtBytes(s.memory_limit)} ({s.memory_percent.toFixed(0)}%)
          </p>
          <p className="text-muted-foreground">
            net ↓ {fmtBytes(s.net_rx_bytes)} ↑ {fmtBytes(s.net_tx_bytes)}
          </p>
        </li>
      ))}
    </ul>
  );
}
