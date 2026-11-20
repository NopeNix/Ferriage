// gen-check: regenerate types.ts from a live peer and fail on
// drift. Used by `npm run gen:check` (CI/pre-commit honesty).
// Needs a peer at $FERRIAGE_PEER_URL (default :18080).
import { execFile } from "node:child_process";
import { readFile } from "node:fs/promises";
import path from "node:path";
import { promisify } from "node:util";

const exec = promisify(execFile);
const TYPES = path.join(process.cwd(), "src", "api", "types.ts");
const before = await readFile(TYPES, "utf8");
await exec("node", ["scripts/gen-types.mjs"], {
  env: { ...process.env },
});
const after = await readFile(TYPES, "utf8");
if (before !== after) {
  console.error("types.ts drifted from the live peer contract — regenerate and commit.");
  process.exit(1);
}
console.log("types.ts matches the live contract");
