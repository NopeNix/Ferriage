import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";
import { OnboardHost } from "./OnboardHost";

describe("OnboardHost", () => {
  it("renders the zero-touch form", () => {
    render(<OnboardHost />);
    expect(screen.getByTestId("onboard-host")).toBeTruthy();
    expect(screen.getByTestId("onboard-address")).toBeTruthy();
    expect(screen.getByTestId("onboard-key")).toBeTruthy();
    expect(screen.getByTestId("onboard-submit")).toBeTruthy();
    // Submit is disabled until address + key are present.
    expect((screen.getByTestId("onboard-submit") as HTMLButtonElement).disabled).toBe(true);
  });
});
