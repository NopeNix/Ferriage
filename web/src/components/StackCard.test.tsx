import { render, screen } from "@testing-library/react";
import { describe, it, expect } from "vitest";
import { StackCard } from "./StackCard";
import type { Stack } from "../api/types";

const STACK: Stack = {
  name: "db",
  compose_file: "/stacks/db/compose.yaml",
  services: [
    { name: "pg", container: "db-pg-1", image: "postgres:17", state: "running", running: true },
    { name: "cache", container: "db-cache-1", image: "redis:8", state: "exited", running: false },
  ],
  running_count: 1,
  container_count: 2,
};

describe("StackCard", () => {
  it("renders name, running badge and per-service state", () => {
    render(<StackCard stack={STACK} />);
    expect(screen.getByTestId("stack-name")).toHaveTextContent("db");
    expect(screen.getByText("1/2 running")).toBeInTheDocument();
  });
});
