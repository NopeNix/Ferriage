import { useState } from "react";
import { ArrowLeft, Download, Rocket, Upload, X } from "lucide-react";
import { api } from "../api/client";
import type { DeployResult, Stack } from "../api/types";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

type Action = "deploy" | "export" | "import" | null;

function shortImage(image: string): string {
  const name = image.split("/").pop() ?? image;
  return name.split(":")[0];
}

export function StackDetail({ stack, onBack, onChanged }: { stack: Stack; onBack: () => void; onChanged: () => void }) {
  const [action, setAction] = useState<Action>(null);
  const [result, setResult] = useState<DeployResult | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [file, setFile] = useState<File | null>(null);

  const run = async (what: Action, fn: () => Promise<DeployResult | Blob | unknown>) => {
    setAction(what);
    setError(null);
    setResult(null);
    try {
      const out = await fn();
      if (what === "export") {
        const blob = out as Blob;
        const url = URL.createObjectURL(blob);
        const a = document.createElement("a");
        a.href = url;
        a.download = `${stack.name}.tar.gz`;
        a.click();
        URL.revokeObjectURL(url);
      } else {
        setResult(out as DeployResult);
      }
      onChanged();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setAction(null);
    }
  };

  return (
    <div>
      <Button variant="ghost" size="sm" onClick={onBack} data-testid="detail-back">
        <ArrowLeft className="size-4" /> All stacks
      </Button>
      <Card className="mt-2">
        <CardHeader>
          <div className="flex items-center justify-between gap-2">
            <CardTitle data-testid="detail-name">{stack.name}</CardTitle>
            <Badge variant="secondary">
              {stack.running_count}/{stack.container_count} running
            </Badge>
          </div>
          {stack.compose_file && <CardDescription className="truncate">{stack.compose_file}</CardDescription>}
          {stack.working_dir && <CardDescription className="truncate">working dir: {stack.working_dir}</CardDescription>}
        </CardHeader>
        <CardContent className="space-y-4">
          <ul className="space-y-2">
            {stack.services.map((s) => (
              <li key={`${s.name}-${s.container}`} className="flex items-center justify-between gap-2 text-sm">
                <span className="flex items-center gap-2">
                  <span className={`inline-block size-2 rounded-full ${s.running ? "bg-emerald-500" : "bg-destructive"}`} />
                  <span className="font-medium">{s.name}</span>
                  <span className="text-muted-foreground">{s.container}</span>
                  <span className="text-muted-foreground">{s.state}</span>
                </span>
                <span className="text-muted-foreground">{shortImage(s.image)}</span>
              </li>
            ))}
          </ul>

          <div className="flex flex-wrap items-center gap-2">
            <Button size="sm" onClick={() => void run("deploy", () => api.deploy(stack.name))} disabled={action !== null} data-testid="deploy-btn">
              <Rocket className="size-4" /> Deploy
            </Button>
            <Button size="sm" variant="outline" onClick={() => void run("export", () => api.exportStack(stack.name))} disabled={action !== null} data-testid="export-btn">
              <Download className="size-4" /> Export snapshot
            </Button>
            <span className="inline-flex items-center gap-2">
              <input
                type="file"
                accept=".tar.gz,.tgz,application/gzip"
                className="hidden"
                data-testid="import-input"
                onChange={(e) => setFile(e.target.files?.[0] ?? null)}
              />
              <Button
                size="sm"
                variant="outline"
                disabled={action !== null}
                onClick={() => document.querySelector<HTMLInputElement>('[data-testid="import-input"]')?.click()}
              >
                <Upload className="size-4" /> {file ? file.name : "Import snapshot…"}
              </Button>
            </span>
            <Button
              size="sm"
              variant="outline"
              disabled={action !== null || !file}
              data-testid="import-btn"
              onClick={() => file && void run("import", () => api.importStack(stack.name, file))}
            >
              Import
            </Button>
            {action && <span className="text-sm text-muted-foreground">{action}ing…</span>}
          </div>

          {result && (
            <div data-testid="deploy-result" className="rounded-md border p-3 text-sm">
              <p className="flex items-center gap-2">
                <Badge variant={result.healthy ? "default" : "destructive"}>{result.healthy ? "healthy" : "unhealthy"}</Badge>
                {result.detail && <span className="text-muted-foreground">{result.detail}</span>}
              </p>
              <ul className="mt-2 space-y-1">
                {result.services.map((s) => (
                  <li key={s.name} className="flex items-center gap-2">
                    <span className="font-medium">{s.name}</span>
                    <span className="text-muted-foreground">
                      {s.running ? "running" : "not running"}
                      {s.has_healthcheck ? ` · health: ${s.health}` : ""}
                    </span>
                  </li>
                ))}
              </ul>
            </div>
          )}
          {error && (
            <p data-testid="detail-error" className="flex items-start gap-2 rounded-md border border-destructive/50 p-3 text-sm">
              <X className="mt-0.5 size-4 text-destructive" />
              {error}
            </p>
          )}
        </CardContent>
      </Card>
    </div>
  );
}
