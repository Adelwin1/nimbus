import { render, screen } from "@testing-library/react";
import { expect, it, vi } from "vitest";
import {
  ReliabilityView,
  duration,
} from "@/components/console/ReliabilityView";
import type { ReliabilityOverview } from "@/types/insights";
vi.mock("recharts", async (importOriginal) => ({
  ...(await importOriginal<typeof import("recharts")>()),
  ResponsiveContainer: () => null,
}));
const empty: ReliabilityOverview = {
  window_start: "2026-10-02T12:00:00Z",
  generated_at: "2026-10-03T12:00:00Z",
  metrics: {
    checks: 0,
    successful: 0,
    success_percent: null,
    average_latency_ms: null,
    p95_latency_ms: null,
  },
  history: [],
  releases: [],
  incidents: [],
  active_incidents: 0,
};
it("keeps an empty workspace distinct from healthy availability", () => {
  render(<ReliabilityView data={empty} />);
  expect(screen.getByText(/No checks in this window/)).toBeVisible();
  expect(screen.queryByText("100.00%")).not.toBeInTheDocument();
  expect(screen.getAllByText("—")).toHaveLength(3);
});
it("links real releases and incidents to their detail routes", () => {
  render(
    <ReliabilityView
      data={{
        ...empty,
        releases: [
          {
            id: "release-1",
            application_name: "Payments",
            version: "v2",
            previous_version: "v1",
            status: "failed",
            kind: "deployment",
            created_at: empty.generated_at,
            duration_seconds: 12,
          },
        ],
        incidents: [
          {
            id: "incident-1",
            application_name: "Payments",
            title: "Release failed",
            status: "open",
            severity: "critical",
            created_at: empty.generated_at,
            duration_seconds: 120,
          },
        ],
      }}
    />,
  );
  expect(screen.getByRole("link", { name: "Payments" })).toHaveAttribute(
    "href",
    "/deployments/release-1",
  );
  expect(screen.getByRole("link", { name: "Release failed" })).toHaveAttribute(
    "href",
    "/incidents/incident-1",
  );
  expect(screen.getByText("v1 → v2")).toBeVisible();
});
it("formats incident and release durations without treating missing time as zero", () => {
  expect(duration(null)).toBe("—");
  expect(duration(8)).toBe("8s");
  expect(duration(125)).toBe("2m 5s");
  expect(duration(3660)).toBe("1h 1m");
});
