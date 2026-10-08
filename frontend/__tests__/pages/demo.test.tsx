import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
vi.mock("recharts", async (importOriginal) => ({
  ...(await importOriginal<typeof import("recharts")>()),
  ResponsiveContainer: () => null,
}));

import DemoPage from "@/app/demo/page";
import HomePage from "@/app/page";

it("makes GitHub the primary entry and keeps the demo optional", () => {
  render(<HomePage />);
  expect(screen.getByRole("button", { name: "Continue with GitHub" })).toBeVisible();
  expect(screen.getByRole("link", { name: "Explore the sample demo" })).toHaveAttribute("href", "/demo");
  expect(screen.getByRole("link", { name: "Log in" })).toHaveAttribute("href", "/login");
});

it("simulates an outage, acknowledgment and recovery without backend requests", async () => {
  const fetchSpy = vi
    .spyOn(globalThis, "fetch")
    .mockRejectedValue(new Error("Backend offline"));
  try {
    const user = userEvent.setup();
    render(<DemoPage />);
    await user.click(screen.getByRole("button", { name: "Simulate outage" }));
    await user.click(screen.getByRole("button", { name: "Incidents" }));
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    expect(screen.getByText("acknowledged", { exact: true })).toBeVisible();
    await user.click(
      screen.getByRole("button", { name: "Recover last healthy version" }),
    );
    expect(await screen.findByText("resolved", { exact: true })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Reset demo" }));
    await user.click(screen.getByRole("button", { name: "Incidents" }));
    expect(screen.getByText(/No incidents yet/)).toBeVisible();
    expect(fetchSpy).not.toHaveBeenCalled();
  } finally {
    fetchSpy.mockRestore();
  }
});

it("retains the healthy version when release verification fails", async () => {
  const user = userEvent.setup();
  render(<DemoPage />);
  await user.click(screen.getByLabelText("Simulate failed verification"));
  await user.click(screen.getByRole("button", { name: "Deploy release" }));
  await waitFor(
    () =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "Last healthy version v2.4.0 retained",
      ),
    { timeout: 3000 },
  );
  expect(screen.getByRole("button", { name: "Deploy release" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Recover application" }));
  await waitFor(() =>
    expect(screen.getByRole("status")).toHaveTextContent("Incident resolved"),
  );
  await user.click(screen.getByLabelText("Simulate failed verification"));
  await user.click(screen.getByRole("button", { name: "Deploy release" }));
  await waitFor(
    () =>
      expect(screen.getByRole("status")).toHaveTextContent(
        "release v2.5.0 passed",
      ),
    { timeout: 3000 },
  );
}, 10000);

it("reset cancels an in-flight release", async () => {
  const user = userEvent.setup();
  render(<DemoPage />);
  await user.click(screen.getByRole("button", { name: "Deploy release" }));
  expect(screen.getByRole("button", { name: "Deploy release" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Reset demo" }));
  expect(screen.getByRole("button", { name: "Deploy release" })).toBeEnabled();
  await new Promise((resolve) => setTimeout(resolve, 800));
  expect(screen.getByRole("status")).toHaveTextContent("Demo reset");
});
