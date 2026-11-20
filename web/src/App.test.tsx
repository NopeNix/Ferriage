import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import App from "./App";

const STACKS = {
  stacks: [
    {
      name: "demo",
      compose_file: "/stacks/demo/compose.yaml",
      services: [
        { name: "app", container: "demo-app-1", image: "nginx:1.27", state: "running", running: true },
        { name: "db", container: "demo-db-1", image: "postgres:17", state: "running", running: true },
      ],
      running_count: 2,
      container_count: 2,
    },
  ],
};

const PEER = { peer_id: "abcdef1234567890", version: "dev", commit: "unknown" };

function mockFetch(impl: (url: string) => unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: URL | Request | string) => {
      const u = String(url);
      if (u.includes("/auth/me")) {
        return { ok: true, status: 200, statusText: "OK", json: async () => ({ sub: "dev@example.com", csrf: "tok" }) };
      }
      if (u.includes("/audit")) {
        return { ok: true, status: 200, statusText: "OK", json: async () => ({ entries: [] }) };
      }
      if (u.includes("/notify")) {
        return { ok: true, status: 200, statusText: "OK", json: async () => ({ enabled: false, channels: 0 }) };
      }
      if (u.includes("/update")) {
        return { ok: true, status: 200, statusText: "OK", json: async () => ({ stacks: [] }) };
      }
      return { ok: true, status: 200, statusText: "OK", json: async () => impl(u) };
    }),
  );
}

// render(<App />) mounts App directly; main.tsx already adds StrictMode
// in production. In tests, per-test stubbing keeps each App's fetch
// isolated, so no cross-test DOM bleed occurs.
describe("App", () => {
  beforeEach(() => vi.unstubAllGlobals());

  it("lists adopted stacks from the peer API", async () => {
    mockFetch((url) => (url.includes("/stacks") ? STACKS : PEER));
    const { unmount } = render(<App />);
    await waitFor(() => expect(screen.getByTestId("stack-card")).toBeInTheDocument());
    expect(screen.getByTestId("stack-name")).toHaveTextContent("demo");
    expect(screen.getByTestId("stack-count")).toHaveTextContent("1 stack adopted");
    expect(screen.getByTestId("peer-badge")).toHaveTextContent("abcdef123456");
    expect(screen.getByTestId("user-badge")).toHaveTextContent("dev@example.com");
    unmount();
  });

  it("opens the stack detail when a card is clicked", async () => {
    mockFetch((url) => (url.includes("/stacks") ? STACKS : PEER));
    const { unmount } = render(<App />);
    await waitFor(() => expect(screen.getByTestId("stack-card")).toBeInTheDocument());
    fireEvent.click(screen.getByTestId("stack-card-wrap"));
    await waitFor(() => expect(screen.getByTestId("detail-name")).toBeInTheDocument());
    expect(screen.getByTestId("detail-name")).toHaveTextContent("demo");
    expect(screen.getByTestId("deploy-btn")).toBeInTheDocument();
    expect(screen.getByTestId("export-btn")).toBeInTheDocument();
    expect(screen.getByTestId("import-btn")).toBeInTheDocument();
    fireEvent.click(screen.getByTestId("detail-back"));
    await waitFor(() => expect(screen.getByTestId("stack-card")).toBeInTheDocument());
    expect(screen.getByTestId("stack-name")).toHaveTextContent("demo");
    unmount();
  });

  it("shows the empty state when there are no stacks", async () => {
    mockFetch((url) => (url.includes("/stacks") ? { stacks: [] } : PEER));
    const { unmount } = render(<App />);
    await waitFor(() => expect(screen.getByTestId("empty")).toBeInTheDocument());
    expect(screen.getByTestId("stack-count")).toHaveTextContent("0 stacks adopted");
    unmount();
  });

  it("shows an offline error when the peer is unreachable", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => {
        throw new Error("network down");
      }),
    );
    const { unmount } = render(<App />);
    await waitFor(() => expect(screen.getByTestId("error")).toBeInTheDocument());
    expect(screen.getByTestId("error")).toHaveTextContent("Could not reach the peer");
    unmount();
  });

  it("shows a login prompt when the API returns 401", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async (url: URL | Request | string) => {
        if (String(url).includes("/auth/me")) {
          return { ok: false, status: 401, statusText: "Unauthorized", json: async () => ({ error: "unauthorized" }) };
        }
        return { ok: false, status: 401, statusText: "Unauthorized", json: async () => ({ error: "unauthorized" }) };
      }),
    );
    const { unmount } = render(<App />);
    await waitFor(() => expect(screen.getByTestId("error")).toBeInTheDocument());
    expect(screen.getByTestId("error")).toHaveTextContent("You are not logged in");
    expect(screen.getByRole("button", { name: /log in/i })).toBeInTheDocument();
    unmount();
  });
});
