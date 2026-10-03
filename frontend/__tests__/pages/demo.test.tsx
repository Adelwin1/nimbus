import { render, screen } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { expect, it, vi } from "vitest";
import DemoPage from "@/app/demo/page";
import HomePage from "@/app/page";

it("takes visitors directly into the demo from the main CTA", () => {
  render(<HomePage />);
  expect(screen.getByRole("link", { name: "Get started" })).toHaveAttribute("href", "/demo");
  expect(screen.getByRole("link", { name: "Log in" })).toHaveAttribute("href", "/login");
});

it("simulates an outage, acknowledgment and recovery without backend requests", async () => {
  const fetchSpy = vi.spyOn(globalThis, "fetch").mockRejectedValue(new Error("Backend offline"));
  try {
    const user = userEvent.setup();
    render(<DemoPage />);
    await user.click(screen.getByRole("button", { name: "Simulate outage" }));
    await user.click(screen.getByRole("button", { name: "Incidents" }));
    await user.click(screen.getByRole("button", { name: "Acknowledge" }));
    expect(screen.getByText("acknowledged", { exact: true })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Recover last healthy version" }));
    expect(screen.getByText("resolved", { exact: true })).toBeVisible();
    await user.click(screen.getByRole("button", { name: "Reset demo" }));
    await user.click(screen.getByRole("button", { name: "Incidents" }));
    expect(screen.getByText(/No incidents yet/)).toBeVisible();
    expect(fetchSpy).not.toHaveBeenCalled();
  } finally { fetchSpy.mockRestore(); }
});

it("retains the healthy version when release verification fails", async () => {
  const user = userEvent.setup();
  render(<DemoPage />);
  await user.click(screen.getByLabelText("Simulate failed verification"));
  await user.click(screen.getByRole("button", { name: "Deploy release" }));
  expect(screen.getByRole("status")).toHaveTextContent("Last healthy version v2.4.0 retained");
  expect(screen.getByRole("button", { name: "Deploy release" })).toBeDisabled();
  await user.click(screen.getByRole("button", { name: "Recover application" }));
  await user.click(screen.getByLabelText("Simulate failed verification"));
  await user.click(screen.getByRole("button", { name: "Deploy release" }));
  expect(screen.getByRole("status")).toHaveTextContent("release v2.5.0 passed");
});
