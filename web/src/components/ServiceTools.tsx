import { useCallback, useEffect, useRef, useState } from "react";
import { Terminal, X } from "lucide-react";
import { api, consoleUrl } from "../api/client";
import type { ExecResult, LogLine } from "../api/types";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";

type Tool = "logs" | "exec" | "console" | null;

interface Props {
  stack: string;
  service: string;
}

const splitCmd = (raw: string): string[] =>
  raw.trim().split(/\s+/).filter(Boolean);

export function ServiceTools({ stack, service }: Props) {
  const [tool, setTool] = useState<Tool>(null);
  const [lines, setLines] = useState<LogLine[]>([]);
  const [logsErr, setLogsErr] = useState<string | null>(null);
  const [cmd, setCmd] = useState("ls -la /");
  const [execOut, setExecOut] = useState<ExecResult | null>(null);
  const [execErr, setExecErr] = useState<string | null>(null);
  const [busy, setBusy] = useState(false);
  const [term, setTerm] = useState("");
  const [termErr, setTermErr] = useState<string | null>(null);
  const wsRef = useRef<WebSocket | null>(null);
  const termRef = useRef<HTMLPreElement | null>(null);

  const loadLogs = useCallback(async () => {
    setLogsErr(null);
    try {
      const res = await api.logs(stack, service, 200);
      setLines(res.lines);
    } catch (err) {
      setLogsErr(err instanceof Error ? err.message : String(err));
    }
  }, [stack, service]);

  useEffect(() => {
    if (tool === "logs") void loadLogs();
  }, [tool, loadLogs]);

  useEffect(() => {
    if (tool !== "logs") return;
    const id = setInterval(() => void loadLogs(), 3000);
    return () => clearInterval(id);
  }, [tool, loadLogs]);

  useEffect(() => {
    if (tool !== "console") return;
    const url = consoleUrl(stack, service);
    const ws = new WebSocket(url);
    wsRef.current = ws;
    let closed = false;
    ws.binaryType = "arraybuffer";
    ws.onmessage = (ev) => {
      if (typeof ev.data === "string" && ev.data.includes('"type"')) {
        try {
          const msg = JSON.parse(ev.data) as { type?: string; message?: string };
          if (msg.type === "error") setTermErr(msg.message ?? "console error");
          return;
        } catch {
          /* fall through: plain text */
        }
      }
      const text =
        typeof ev.data === "string"
          ? ev.data
          : new TextDecoder().decode(ev.data as ArrayBuffer);
      setTerm((t) => t + text);
    };
    ws.onerror = () => {
      if (!closed) setTermErr("console connection failed");
    };
    return () => {
      closed = true;
      try {
        ws.send(JSON.stringify({ type: "close" }));
      } catch {
        /* already closed */
      }
      ws.close();
      wsRef.current = null;
    };
  }, [tool, stack, service]);

  useEffect(() => {
    const el = termRef.current;
    if (el) el.scrollTop = el.scrollHeight;
  }, [term]);

  const runExec = async () => {
    setBusy(true);
    setExecErr(null);
    setExecOut(null);
    try {
      setExecOut(await api.exec(stack, service, splitCmd(cmd)));
    } catch (err) {
      setExecErr(err instanceof Error ? err.message : String(err));
    } finally {
      setBusy(false);
    }
  };

  const sendTerm = (text: string) => {
    const ws = wsRef.current;
    if (ws && ws.readyState === WebSocket.OPEN) ws.send(text);
  };

  return (
    <div className="mt-4 border-t pt-4" data-testid="service-tools">
      <div className="flex flex-wrap items-center gap-2">
        <span className="text-sm font-medium">{service}</span>
        {(["logs", "exec", "console"] as Tool[]).map((t) =>
          t ? (
            <Button
              key={t}
              size="sm"
              variant={tool === t ? "default" : "outline"}
              onClick={() => setTool(tool === t ? null : t)}
              data-testid={`tool-${t}-btn`}
            >
              {t === "console" && <Terminal className="size-4" />}
              {t[0].toUpperCase() + t.slice(1)}
            </Button>
          ) : null,
        )}
        {tool === "logs" && (
          <Button size="sm" variant="ghost" onClick={() => void loadLogs()}>
            Refresh
          </Button>
        )}
      </div>

      {tool === "logs" && (
        <div className="mt-3">
          {logsErr && (
            <p data-testid="logs-error" className="text-sm text-destructive">
              {logsErr}
            </p>
          )}
          <pre
            data-testid="logs-view"
            className="max-h-64 overflow-auto rounded-md border bg-muted/40 p-3 font-mono text-xs leading-relaxed"
          >
            {lines.length === 0 && !logsErr && <span className="text-muted-foreground">No log lines yet.</span>}
            {lines.map((l, i) => (
              <div key={i} className="whitespace-pre-wrap break-all">
                {l.ts && <span className="text-muted-foreground">{l.ts} </span>}
                <span className={l.stream === "stderr" ? "text-destructive" : undefined}>{l.message}</span>
              </div>
            ))}
          </pre>
        </div>
      )}

      {tool === "exec" && (
        <div className="mt-3" data-testid="exec-panel">
          <div className="flex gap-2">
            <input
              className="h-9 flex-1 rounded-md border bg-background px-3 font-mono text-sm"
              value={cmd}
              onChange={(e) => setCmd(e.target.value)}
              onKeyDown={(e) => e.key === "Enter" && void runExec()}
              data-testid="exec-input"
              aria-label="command"
            />
            <Button size="sm" onClick={() => void runExec()} disabled={busy} data-testid="exec-run">
              Run
            </Button>
          </div>
          {execErr && (
            <p data-testid="exec-error" className="mt-2 text-sm text-destructive">
              {execErr}
            </p>
          )}
          {execOut && (
            <div className="mt-2" data-testid="exec-output">
              <p className="flex items-center gap-2 text-sm">
                <Badge variant={execOut.exit_code === 0 ? "default" : "destructive"}>
                  exit {execOut.exit_code}
                </Badge>
                {execOut.truncated && <span className="text-muted-foreground">output truncated</span>}
              </p>
              <pre className="mt-2 max-h-64 overflow-auto whitespace-pre-wrap break-all rounded-md border bg-muted/40 p-3 font-mono text-xs">
                {execOut.output || "(no output)"}
              </pre>
            </div>
          )}
        </div>
      )}

      {tool === "console" && (
        <div className="mt-3" data-testid="console-panel">
          {termErr && (
            <p data-testid="console-error" className="mb-2 text-sm text-destructive">
              {termErr}
            </p>
          )}
          <pre
            ref={termRef}
            data-testid="console-view"
            className="h-64 overflow-auto rounded-md border bg-black p-3 font-mono text-xs leading-relaxed text-green-300"
          >
            {term || "Connecting… (type to interact)"}
          </pre>
          <div className="mt-2 flex gap-2">
            <input
              className="h-9 flex-1 rounded-md border bg-background px-3 font-mono text-sm"
              placeholder="type + Enter (\\n sent)"
              onKeyDown={(e) => {
                if (e.key === "Enter") {
                  sendTerm(e.currentTarget.value + "\n");
                  e.currentTarget.value = "";
                }
              }}
              data-testid="console-input"
              aria-label="console input"
            />
            <Button size="sm" variant="outline" onClick={() => setTerm("")}>
              <X className="size-4" /> Clear
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
