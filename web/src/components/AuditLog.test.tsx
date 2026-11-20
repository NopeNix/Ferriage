import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

const auditMock = vi.fn();

vi.mock("../api/client", () => ({
  api: {
    audit: (opts?: unknown) => auditMock(opts),
  },
}));

import { AuditLog } from "./AuditLog";

describe("AuditLog", () => {
  it("renders entries newest-first with actor, action, target, and status", async () => {
    auditMock.mockResolvedValue({
      entries: [
        { ts: "2026-11-20T10:00:00Z", actor: "alice@example.com", action: "deploy", target: "demo", status: 200 },
        { ts: "2026-11-20T11:00:00Z", actor: "bob@example.com", action: "exec", target: "demo/app", status: 502 },
      ],
    });
    render(<AuditLog />);
    expect(screen.getByTestId("audit-log")).toBeTruthy();
    await waitFor(() => expect(screen.getAllByTestId("audit-entry")).toHaveLength(2));
    const entries = screen.getAllByTestId("audit-entry");
    expect(entries[0]).toHaveTextContent("alice@example.com");
    expect(entries[0]).toHaveTextContent("deploy");
    expect(entries[0]).toHaveTextContent("demo");
    expect(entries[0]).toHaveTextContent("200");
    expect(entries[1]).toHaveTextContent("502");
  });

  it("shows the empty state when nothing has been recorded", async () => {
    auditMock.mockResolvedValue({ entries: [] });
    render(<AuditLog />);
    await waitFor(() => expect(screen.getByTestId("audit-empty")).toBeTruthy());
  });

  it("shows an error state when the peer fails", async () => {
    auditMock.mockRejectedValue(new Error("peer down"));
    render(<AuditLog />);
    await waitFor(() => expect(screen.getByTestId("audit-error")).toBeTruthy());
  });
});
