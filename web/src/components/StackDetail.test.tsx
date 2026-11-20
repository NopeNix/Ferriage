import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { StackDetail } from "./StackDetail";
import type { Stack } from "../api/types";

const STACK: Stack = {
  name: "demo",
  compose_file: "/stacks/demo/compose.yaml",
  services: [
    { name: "app", container: "demo-app-1", image: "alpine:3.20", state: "running", running: true },
  ],
  running_count: 1,
  container_count: 1,
};

function mockFetch(impl: (url: string) => unknown) {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: URL | Request | string) => {
      const u = String(url);
      if (u.includes("/auth/me")) {
        return { ok: true, status: 200, statusText: "OK", json: async () => ({ sub: "dev@example.com", csrf: "tok" }) };
      }
      return impl(u);
    }),
  );
}

describe("StackDetail", () => {
  beforeEach(() => vi.unstubAllGlobals());

  it("deploys and reports the health gate result", async () => {
    mockFetch((url) => {
      if (url.includes("/deploy")) {
        return {
          ok: true,
          status: 200,
          statusText: "OK",
          json: async () => ({ stack: "demo", healthy: true, services: [{ name: "app", health: "healthy", has_healthcheck: true, running: true }] }),
        };
      }
      return { ok: true, status: 200, statusText: "OK", json: async () => ({}) };
    });
    const onChanged = vi.fn();
    const { unmount } = render(<StackDetail stack={STACK} onBack={() => {}} onChanged={onChanged} />);
    await waitFor(() => expect(screen.getByTestId("detail-name")).toHaveTextContent("demo"));
    fireEvent.click(screen.getByTestId("deploy-btn"));
    await waitFor(() => expect(screen.getByTestId("deploy-result")).toBeInTheDocument());
    expect(screen.getByTestId("deploy-result")).toHaveTextContent("healthy");
    expect(onChanged).toHaveBeenCalled();
    unmount();
  });

  it("exports a snapshot blob to a download", async () => {
    const click = vi.fn();
    const createEl = document.createElement.bind(document);
    vi.spyOn(document, "createElement").mockImplementation(((tag: string) => {
      const el = createEl(tag);
      if (tag === "a") el.click = click;
      return el;
    }) as typeof document.createElement);
    mockFetch((url) => {
      if (url.includes("/export")) {
        return { ok: true, status: 200, statusText: "OK", blob: async () => new Blob(["x"]) };
      }
      return { ok: true, status: 200, statusText: "OK", json: async () => ({}) };
    });
    const { unmount } = render(<StackDetail stack={STACK} onBack={() => {}} onChanged={() => {}} />);
    fireEvent.click(screen.getByTestId("export-btn"));
    await waitFor(() => expect(click).toHaveBeenCalled());
    unmount();
  });

  it("shows an error when the peer rejects the deploy", async () => {
    mockFetch(() => ({
      ok: false,
      status: 404,
      statusText: "Not Found",
      json: async () => ({ error: "stack not found" }),
    }));
    const { unmount } = render(<StackDetail stack={STACK} onBack={() => {}} onChanged={() => {}} />);
    fireEvent.click(screen.getByTestId("deploy-btn"));
    await waitFor(() => expect(screen.getByTestId("detail-error")).toBeInTheDocument());
    expect(screen.getByTestId("detail-error")).toHaveTextContent("404");
    expect(screen.getByTestId("detail-error")).toHaveTextContent("stack not found");
    unmount();
  });
});
