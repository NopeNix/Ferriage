import { useCallback, useEffect, useState } from "react";
import { Bell, Loader2, Send } from "lucide-react";
import { api } from "../api/client";
import type { NotifyStatus as NotifyStatusT } from "../api/types";
import { Button } from "./ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card";

type TestState = { kind: "idle" } | { kind: "sending" } | { kind: "sent" } | { kind: "error"; message: string };

// NotifyStatus is the read-only Shoutrrr card (ADR 0007, option
// A): it shows whether the peer has channels configured and the
// outcome of the last send. URLs are env-only on the peer
// (FERRIAGE_NOTIFY_URLS) and never cross the API, so there is
// no settings form here — only status + a test send.
export function NotifyStatus() {
  const [status, setStatus] = useState<NotifyStatusT | null>(null);
  const [test, setTest] = useState<TestState>({ kind: "idle" });

  const load = useCallback(async () => {
    try {
      setStatus(await api.notifyStatus());
    } catch {
      setStatus(null);
    }
  }, []);

  useEffect(() => {
    void load();
  }, [load]);

  async function sendTest() {
    setTest({ kind: "sending" });
    try {
      await api.notifyTest();
      setTest({ kind: "sent" });
      await load();
    } catch (err) {
      setTest({ kind: "error", message: err instanceof Error ? err.message : String(err) });
    }
  }

  const enabled = status?.enabled ?? false;

  return (
    <Card data-testid="notify-status">
      <CardHeader>
        <CardTitle className="flex items-center gap-2">
          <Bell className="size-5" />
          Notifications (Shoutrrr)
        </CardTitle>
        <CardDescription>
          {status === null
            ? "Could not load notification status."
            : enabled
              ? `${status.channels} channel${status.channels === 1 ? "" : "s"} configured via FERRIAGE_NOTIFY_URLS (deploy + onboard events).`
              : "Disabled — set FERRIAGE_NOTIFY_URLS on the peer to enable deploy/onboard notifications."}
        </CardDescription>
      </CardHeader>
      <CardContent className="grid gap-3">
        {status && status.last_event && (
          <p data-testid="notify-last" className="text-sm text-muted-foreground">
            Last: {status.last_event}
            {status.last_at ? ` at ${status.last_at}` : ""}
            {status.last_error ? ` — error: ${status.last_error}` : ""}
          </p>
        )}
        <div className="flex items-center gap-3">
          <Button variant="outline" size="sm" onClick={() => void sendTest()} disabled={!enabled || test.kind === "sending"} data-testid="notify-test">
            {test.kind === "sending" ? <Loader2 className="size-4 animate-spin" /> : <Send className="size-4" />}
            Send test
          </Button>
          {test.kind === "sent" && (
            <span data-testid="notify-sent" className="text-sm text-emerald-600">
              Test notification sent.
            </span>
          )}
          {test.kind === "error" && (
            <span data-testid="notify-error" className="text-sm text-destructive">
              {test.message}
            </span>
          )}
        </div>
      </CardContent>
    </Card>
  );
}
