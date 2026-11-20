// Vendors monaco-editor's min/ build into web/dist/monaco so the
// compose editor loads offline from the UI container itself —
// no CDN fetch at runtime (CONCEPT: no external dependency in
// the hot path; the homelab LAN may not reach jsDelivr).
// Runs as `npm run postbuild` after vite build.
import { cp, rm } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const SRC = path.join(HERE, "..", "node_modules", "monaco-editor", "min");
const DST = path.join(HERE, "..", "dist", "monaco");

await rm(DST, { recursive: true, force: true });
await cp(SRC, DST, { recursive: true });
console.log(`vendored monaco-editor -> dist/monaco`);
