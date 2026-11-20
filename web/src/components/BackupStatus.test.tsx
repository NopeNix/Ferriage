import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi, beforeEach } from "vitest";
import { BackupStatus } from "./BackupStatus";

vi.mock("../api/client", () => ({
  api: {
    backup: vi.fn(),
    runBackup: vi.fn(),
    forgetBackups: vi.fn(),
  },
}));

import { api } from "../api/client";

const mocked = vi.mocked(api);

describe("BackupStatus", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  it("renders repo status with password_set, no password", async () => {
    mocked.backup.mockResolvedValue({
      status: {
        repo: "/stacks/.ferriage/restic-repo",
        initialized: true,
        snapshots: 1,
        password_set: true,
        restic: "restic",
      },
      snapshots: [
        { id: "abc123def456789", time: "2026-11-20T10:00:00Z", tags: ["ferriage", "stack"], size: 42 },
      ],
    } as never);
    render(<BackupStatus />);
    await waitFor(() => expect(screen.getByTestId("backup-status")).toBeInTheDocument());
    expect(screen.getByText(/1 snapshot/)).toBeInTheDocument();
    expect(screen.getByTestId("backup-snapshot")).toBeInTheDocument();
    expect(document.body.textContent).not.toMatch(/password["']\s*:/);
  });

  it("handles uninitialized repo", async () => {
    mocked.backup.mockResolvedValue({
      status: { repo: "/stacks/.ferriage/restic-repo", initialized: false, snapshots: 0, password_set: false },
      snapshots: [],
    } as never);
    render(<BackupStatus />);
    await waitFor(() => expect(screen.getByText(/Repo not initialized/)).toBeInTheDocument());
  });
});
