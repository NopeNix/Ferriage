import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ServiceTools } from "./ServiceTools";

function mockFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: URL | Request | string) => {
      const u = String(url);
      if (u.includes("/auth/me")) return { ok: true, status: 200, json: async () => ({ sub: "dev@example.com", csrf: "tok" }) };
      if (u.includes("/logs")) return { ok: true, status: 200, json: async () => ({ container: "cid", lines: [{ stream: "stdout", message: "hello logs" }], truncated: false }) };
      if (u.includes("/exec")) return { ok: true, status: 200, json: async () => ({ exec_id: "e1", running: false, exit_code: 0, output: "file1\nfile2", truncated: false }) };
      return { ok: true, status: 200, json: async () => ({}) };
    }),
  );
}

describe("ServiceTools", () => {
  beforeEach(() => vi.unstubAllGlobals());

  it("shows log lines when Logs is opened", async () => {
    mockFetch();
    render(<ServiceTools stack="demo" service="app" />);
    fireEvent.click(screen.getByTestId("tool-logs-btn"));
    await waitFor(() => expect(screen.getByTestId("logs-view")).toBeInTheDocument());
    await waitFor(() => expect(screen.getByTestId("logs-view")).toHaveTextContent("hello logs"));
  });

  it("runs an exec command and renders its output", async () => {
    mockFetch();
    render(<ServiceTools stack="demo" service="app" />);
    fireEvent.click(screen.getByTestId("tool-exec-btn"));
    fireEvent.change(screen.getByTestId("exec-input"), { target: { value: "ls /" } });
    fireEvent.click(screen.getByTestId("exec-run"));
    await waitFor(() => expect(screen.getByTestId("exec-output")).toBeInTheDocument());
    expect(screen.getByTestId("exec-output")).toHaveTextContent("file1");
  });

  it("opens the console panel", async () => {
    mockFetch();
    vi.stubGlobal(
      "WebSocket",
      class {
        static OPEN = 1;
        readyState = 1;
        binaryType = "";
        onmessage: ((ev: { data: string }) => void) | null = null;
        onerror: (() => void) | null = null;
        send() {}
        close() {}
      },
    );
    render(<ServiceTools stack="demo" service="app" />);
    fireEvent.click(screen.getByTestId("tool-console-btn"));
    expect(screen.getByTestId("console-panel")).toBeInTheDocument();
  });
});
