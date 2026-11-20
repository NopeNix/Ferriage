import { render, screen, waitFor } from "@testing-library/react";
import { beforeEach, expect, test, vi } from "vitest";
import { UpdateStatus } from "./UpdateStatus";
import { api } from "../api/client";

vi.mock("../api/client", () => ({
  api: {
    update: vi.fn(),
    setUpdatePolicy: vi.fn(),
  },
}));

beforeEach(() => {
  vi.mocked(api.update).mockResolvedValue({
    stacks: [
      {
        stack: "demo",
        state: "available",
        policy: "poll",
        stateful: true,
        checked_at: "2026-11-20T18:00:00Z",
        services: [{ service: "app", image: "nginx:1", outcome: "available" }],
      },
    ],
  });
});

test("renders policy, state, and stateful flag", async () => {
  render(<UpdateStatus />);
  await waitFor(() => expect(screen.getByTestId("update-stack")).toBeInTheDocument());
  expect(screen.getByText("demo")).toBeInTheDocument();
  expect(screen.getAllByText("poll").length).toBeGreaterThan(0);
  expect(screen.getByText(/stateful/)).toBeInTheDocument();
  expect(screen.getAllByText(/available/).length).toBeGreaterThan(0);
});

test("shows empty state", async () => {
  vi.mocked(api.update).mockResolvedValue({ stacks: [] });
  render(<UpdateStatus />);
  await waitFor(() => expect(screen.getByTestId("update-empty")).toBeInTheDocument());
});
