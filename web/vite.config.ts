import { defineConfig } from "vitest/config";
import react from "@vitejs/plugin-react";
import tailwindcss from "@tailwindcss/vite";

// Dev: Vite proxies /api to the peer on :8080, so `npm run dev` works
// against a local peer with no BFF. The production BFF (bff/index.mjs)
// does the same proxying and also serves dist/.
export default defineConfig({
  plugins: [react(), tailwindcss()],
  server: {
    proxy: {
      "/api": { target: "http://localhost:8080", rewrite: (p) => p.replace(/^\/api/, "") },
    },
  },
  test: {
    environment: "jsdom",
    setupFiles: ["./src/tests/setup.ts"],
    // bff/*.test.mjs uses node:test, not vitest (run via `node --test`).
    exclude: ["node_modules", "dist", "bff/**"],
  },
});
