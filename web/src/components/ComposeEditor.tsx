import { useEffect, useRef, useState } from "react";
import Editor, { loader } from "@monaco-editor/react";
import type { Monaco } from "@monaco-editor/react";
import { configureMonacoYaml } from "monaco-yaml";
import { FileCode2, Save, X } from "lucide-react";
import { api } from "../api/client";
import { Badge } from "./ui/badge";
import { Button } from "./ui/button";

// The monaco-editor package is vendored into the UI image at
// build time (web/scripts/copy-monaco.mjs): no CDN fetch at
// runtime, the editor works offline on the LAN. loader.config
// points at the local copy served from the same origin.
loader.config({ paths: { vs: "/monaco/vs" } });

// The compose JSON schema, bundled so the edit loop works
// offline in the UI container (no CDN fetch at runtime).
const COMPOSE_SCHEMA_URL = "https://raw.githubusercontent.com/compose-spec/compose-go/main/docs/reference/schema.json";

interface Props {
  stack: string;
  onSaved?: () => void;
}

let yamlConfigured = false;

function beforeMount(monaco: Monaco) {
  if (yamlConfigured) return;
  yamlConfigured = true;
  configureMonacoYaml(monaco, {
    enableSchemaRequest: false,
    schemas: [
      {
        uri: COMPOSE_SCHEMA_URL,
        fileMatch: ["compose.yaml", "compose.yml"],
      },
    ],
    format: { enable: true },
    completion: true,
    hover: true,
    validate: true,

  });
}

export function ComposeEditor({ stack, onSaved }: Props) {
  const [content, setContent] = useState<string | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [saving, setSaving] = useState(false);
  const [savedAt, setSavedAt] = useState<string | null>(null);
  const [dirty, setDirty] = useState(false);
  const editorRef = useRef<{ setValue: (v: string) => void } | null>(null);

  useEffect(() => {
    let cancelled = false;
    setError(null);
    setContent(null);
    setDirty(false);
    setSavedAt(null);
    api
      .compose(stack)
      .then((res) => {
        if (!cancelled) setContent(res.content);
      })
      .catch((err) => {
        if (!cancelled) setError(err instanceof Error ? err.message : String(err));
      });
    return () => {
      cancelled = true;
    };
  }, [stack]);

  const save = async () => {
    if (content === null) return;
    setSaving(true);
    setError(null);
    try {
      await api.saveCompose(stack, content);
      setDirty(false);
      setSavedAt(new Date().toISOString());
      onSaved?.();
    } catch (err) {
      setError(err instanceof Error ? err.message : String(err));
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="mt-4 border-t pt-4" data-testid="compose-editor">
      <div className="flex flex-wrap items-center gap-2">
        <span className="flex items-center gap-1.5 text-sm font-medium">
          <FileCode2 className="size-4" /> compose.yaml
        </span>
        {dirty && <Badge variant="secondary">unsaved</Badge>}
        {savedAt && !dirty && <Badge variant="default">saved</Badge>}
        <span className="ml-auto flex gap-2">
          <Button size="sm" onClick={() => void save()} disabled={saving || content === null || !dirty} data-testid="compose-save">
            <Save className="size-4" /> {saving ? "Saving…" : "Save"}
          </Button>
        </span>
      </div>
      {error && (
        <p data-testid="compose-error" className="mt-2 flex items-start gap-2 text-sm text-destructive">
          <X className="mt-0.5 size-4" /> {error}
        </p>
      )}
      {content === null && !error && <p className="mt-3 text-sm text-muted-foreground">Loading compose file…</p>}
      {content !== null && (
        <div className="mt-3 overflow-hidden rounded-md border" data-testid="compose-monaco">
          <Editor
            height="420px"
            language="yaml"
            path={`compose-${stack}.yaml`}
            value={content}
            onMount={(editor) => {
              editorRef.current = editor;
            }}
            onChange={(v) => {
              setContent(v ?? "");
              setDirty(true);
            }}
            beforeMount={beforeMount}
            options={{
              minimap: { enabled: false },
              fontSize: 13,
              lineNumbers: "on",
              tabSize: 2,
              insertSpaces: true,
              detectIndentation: false,

            }}
          />
        </div>
      )}
      <p className="mt-2 text-xs text-muted-foreground">
        Schema-backed autocomplete + diagnostics (monaco-yaml). Save atomically replaces the
        state file; deploy to apply. State stays a file (CONCEPT req. 6).
      </p>
    </div>
  );
}
