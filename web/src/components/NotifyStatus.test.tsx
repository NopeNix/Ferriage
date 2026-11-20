import { render, screen, waitFor } from "@testing-library/react";
import { describe, expect, it, vi } from "vitest";

vi.mock("../api/client", () => ({
  api: {
    notifyStatus: () => Promise.resolve({ enabled: false, channels: 0 }),
    notifyTest: () => Promise.resolve({ sent: true, event: "notify.test" }),
  },
}));

import { NotifyStatus } from "./NotifyStatus";

describe("NotifyStatus", () => {
  it("shows disabled state and disables the test button", async () => {
    render(<NotifyStatus />);
    expect(screen.getByTestId("notify-status")).toBeTruthy();
    await waitFor(() => expect(screen.queryByText(/Disabled/)).toBeTruthy());
    expect((screen.getByTestId("notify-test") as HTMLButtonElement).disabled).toBe(true);
  });
});
