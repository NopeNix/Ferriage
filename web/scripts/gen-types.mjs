// Generates web/src/api/types.ts from the peer's OpenAPI contract
// (CONCEPT: API-first). Source: GET <peer>/openapi.json, or a local
// copy passed as argv[2]. Run against a live peer:
//
//   node web/scripts/gen-types.mjs            # needs FERRIAGE_PEER_URL
//   node web/scripts/gen-types.mjs doc.json   # from a saved copy
//
// The output is committed; `npm run gen:check` regenerates from a
// live peer and fails on drift. The commit is the source of truth —
// if types.ts disagrees with the contract, regenerate, don't edit.
import { readFile, writeFile } from "node:fs/promises";
import path from "node:path";
import { fileURLToPath } from "node:url";

const HERE = path.dirname(fileURLToPath(import.meta.url));
const OUT = path.join(HERE, "..", "src", "api", "types.ts");

const arg = process.argv[2];
const doc = JSON.parse(
  arg ? await readFile(arg, "utf8") : await (await fetch(`${(process.env.FERRIAGE_PEER_URL || "http://localhost:18080").replace(/\/$/, "")}/openapi.json`)).text(),
);

const schemas = doc.components.schemas;
const emitted = new Set();
const chunks = [];

const ORDER = ["Error", "Health", "Peer", "Service", "Stack", "Stacks", "DeploySvc", "DeployResult", "VolumeDesc", "SnapshotManifest", "SyncEntry", "SyncManifest", "OnboardStep", "OnboardResponse"];

function refName(ref) {
  const m = /^#\/components\/schemas\/(.+)$/.exec(ref);
  if (!m) throw new Error(`unsupported $ref: ${ref}`);
  return m[1];
}

function typeOf(schema) {
  const ref = schema.$ref;
  if (ref) return refName(ref);
  switch (schema.type) {
    case "string":
      return schema.format === "binary" ? "string" : "string";
    case "integer":
    case "number":
      return "number";
    case "boolean":
      return "boolean";
    case "array":
      return `${typeOf(schema.items)}[]`;
    case "object":
      return "Record<string, unknown>";
    default:
      return "unknown";
  }
}

// Emit interface for one schema; inline object properties become
// anonymous nested interfaces is overkill — the contract's objects
// are all named schemas except arrays of them.
function emit(name) {
  if (emitted.has(name)) return;
  emitted.add(name);
  const schema = schemas[name];
  if (!schema) throw new Error(`unknown schema ${name}`);
  // Recurse first so dependencies are emitted before use.
  for (const prop of Object.values(schema.properties ?? {})) {
    if (prop.$ref) emit(refName(prop.$ref));
    if (prop.type === "array" && prop.items?.$ref) emit(refName(prop.items.$ref));
  }
  const required = new Set(schema.required ?? []);
  const props = Object.entries(schema.properties ?? {}).map(([key, prop]) => {
    const optional = required.has(key) ? "" : "?";
    return `  ${key}${optional}: ${typeOf(prop)};`;
  });
  chunks.push(`export interface ${name} {\n${props.join("\n")}\n}`);
}

for (const name of ORDER) {
  if (schemas[name]) emit(name);
}
for (const name of Object.keys(schemas).sort()) emit(name);

const header = `// GENERATED from the peer OpenAPI contract — never edit by hand.
// Source: internal/api (GET /openapi.json). Regenerate with
//   node web/scripts/gen-types.mjs
// or keep it honest with: npm run gen:check
//
// Field names and nullability mirror the Go structs; required[]
// in the contract decides which fields are optional here.

`;

await writeFile(OUT, header + chunks.join("\n\n") + "\n");
console.log(`wrote ${path.relative(process.cwd(), OUT)} (${chunks.length} interfaces)`);
