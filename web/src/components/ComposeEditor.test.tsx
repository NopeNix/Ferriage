import { render, screen, waitFor, fireEvent } from "@testing-library/react";
import { describe, it, expect, vi, beforeEach } from "vitest";
import { ComposeEditor } from "./ComposeEditor";

function mockFetch() {
  vi.stubGlobal(
    "fetch",
    vi.fn(async (url: URL | Request | string) => {
      const u = String(url);
      if (u.includes("/auth/me")) return { ok: true, status: 200, json: async () => ({ sub: "dev@example.com", csrf: "tok" }) };
      if (u.includes("/compose")) return { ok: true, status: 200, json: async () => ({ name: "demo", file: "compose.yaml", content: "services: {}\n" }) };
      return { ok: true, status: 200, json: async () => ({}) };
    }),
  );
}

describe("ComposeEditor", () => {
  beforeEach(() => vi.unstubAllGlobals());

  it("renders the compose editor with the save control", async () => {
    mockFetch();
    render(<ComposeEditor stack="demo" />);
    await waitFor(() => expect(screen.getByTestId("compose-monaco")).toBeInTheDocument());
    expect(screen.getByTestId("compose-save")).toBeInTheDocument();
  });

  it("shows a fetch error inline", async () => {
    vi.stubGlobal(
      "fetch",
      vi.fn(async () => ({ ok: false, status: 404, json: async () => ({ error: "compose file not found" }) })),
    );
    render(<ComposeEditor stack="ghost" />);
    await waitFor(() => expect(screen.getByTestId("compose-error")).toBeInTheDocument());
    expect(screen.getByTestId("compose-error")).toHaveTextContent("404");
    fireEvent.click(screen.getByTestId("compose-save"));
  });
});
