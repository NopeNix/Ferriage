import { useState } from "react";
import { Check, Loader2, Server, X } from "lucide-react";
import { api } from "../api/client";
import type { OnboardResponse } from "../api/types";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

type Status = { kind: "idle" } | { kind: "running" } | { kind: "done"; result: OnboardResponse } | { kind: "error"; message: string };

async function fileText(file: File): Promise<string> {
  return await file.text();
}

export function OnboardHost() {
  const [address, setAddress] = useState("");
  const [user, setUser] = useState("root");
  const [keyFile, setKeyFile] = useState<File | null>(null);
  const [image, setImage] = useState("");
  const [peerUrl, setPeerUrl] = useState("");
  const [port, setPort] = useState("");
  const [status, setStatus] = useState<Status>({ kind: "idle" });
  const running = status.kind === "running";

  async function submit(e: React.FormEvent) {
    e.preventDefault();
    if (!keyFile) return;
    setStatus({ kind: "running" });
    try {
      const privateKey = await fileText(keyFile);
      const result = await api.onboardHost({
        address: address.trim(),
        user: user.trim() || undefined,
        privateKey,
        image: image.trim() || undefined,
        peerUrl: peerUrl.trim() || undefined,
        port: port.trim() ? Number(port) : undefined,
      });
      if (result.error) {
        setStatus({ kind: "error", message: result.error });
      } else {
        setStatus({ kind: "done", result });
      }
    } catch (err) {
      setStatus({ kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
  }

  const steps = status.kind === "done" ? status.result.steps : null;

  return (
    <Card data-testid="onboard-host">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Server className="size-5" />
          Add a host (SSH onboarding)
        </CardTitle>
        <CardDescription>
          Zero-touch: address + key → Docker check → one Ferriage container. The key is sent once and kept in memory
          only (ADR 0006). Docker is installed if missing — confirm the host is yours before running this.
        </CardDescription>
      </CardHeader>
      <CardContent>
        <form onSubmit={(e) => void submit(e)} className="grid gap-3 sm:grid-cols-2">
          <label className="grid gap-1 text-sm">
            <span className="font-medium">Address (host or host:port)</span>
            <input
              className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="192.168.1.10 or vps.example.com:22"
              value={address}
              onChange={(e) => setAddress(e.target.value)}
              required
              disabled={running}
              data-testid="onboard-address"
            />
          </label>
          <label className="grid gap-1 text-sm">
            <span className="font-medium">SSH user</span>
            <input
              className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={user}
              onChange={(e) => setUser(e.target.value)}
              disabled={running}
              data-testid="onboard-user"
            />
          </label>
          <label className="grid gap-1 text-sm">
            <span className="font-medium">SSH private key (.pem)</span>
            <input
              type="file"
              accept=".pem,.key"
              className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              onChange={(e) => setKeyFile(e.target.files?.[0] ?? null)}
              required
              disabled={running}
              data-testid="onboard-key"
            />
          </label>
          <label className="grid gap-1 text-sm">
            <span className="font-medium">Image (default nopenix/ferriage:latest)</span>
            <input
              className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={image}
              onChange={(e) => setImage(e.target.value)}
              disabled={running}
              data-testid="onboard-image"
            />
          </label>
          <label className="grid gap-1 text-sm">
            <span className="font-medium">Mesh peer URL (this peer, for sync)</span>
            <input
              className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              placeholder="http://10.0.0.1:8080"
              value={peerUrl}
              onChange={(e) => setPeerUrl(e.target.value)}
              disabled={running}
              data-testid="onboard-peer-url"
            />
          </label>
          <label className="grid gap-1 text-sm">
            <span className="font-medium">Peer port (default 8080)</span>
            <input
              type="number"
              min={1}
              max={65535}
              className="rounded-md border border-input bg-background px-3 py-2 text-sm"
              value={port}
              onChange={(e) => setPort(e.target.value)}
              disabled={running}
              data-testid="onboard-port"
            />
          </label>
          <div className="sm:col-span-2">
            <Button type="submit" disabled={running || !address.trim() || !keyFile} data-testid="onboard-submit">
              {running ? (
                <>
                  <Loader2 className="size-4 animate-spin" /> Onboarding…
                </>
              ) : (
                "Onboard host"
              )}
            </Button>
          </div>
        </form>

        {status.kind === "error" && (
          <p data-testid="onboard-error" className="mt-4 rounded-lg border border-destructive/50 p-3 text-sm text-destructive">
            {status.message}
          </p>
        )}
        {steps && (
          <ol data-testid="onboard-steps" className="mt-4 grid gap-1 text-sm">
            {steps.map((s) => (
              <li key={s.name} className="flex items-center gap-2">
                {s.ok ? <Check className="size-4 text-emerald-600" /> : <X className="size-4 text-destructive" />}
                <span className="font-medium">{s.name}</span>
                {s.detail && <span className="text-muted-foreground">{s.detail}</span>}
              </li>
            ))}
          </ol>
        )}
        {status.kind === "done" && (
          <p data-testid="onboard-success" className="mt-4 text-sm text-emerald-600">
            {status.result.address} onboarded — docker {status.result.docker}, compose {status.result.compose}, healthy{" "}
            {String(status.result.healthy)}.
          </p>
        )}
      </CardContent>
    </Card>
  );
}
