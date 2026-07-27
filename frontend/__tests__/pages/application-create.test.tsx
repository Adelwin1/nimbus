import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import NewApplicationPage from "@/app/apps/new/page";
import { APIError } from "@/lib/api";

const mocks = vi.hoisted(() => ({
  createApplication: vi.fn(),
  push: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    push: mocks.push,
  }),
}));

vi.mock("@/components/auth/ProtectedRoute", () => ({
  ProtectedRoute: ({ children }: { children: ReactNode }) => children,
}));

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();

  return {
    ...actual,
    createApplication: mocks.createApplication,
  };
});

const createdApplication = {
  id: "app-123",
  name: "Nimbus API",
  description: "",
  application_url: "https://api.example.com",
  health_url: "https://api.example.com/health",
  repository_url: null,
  environment: "production" as const,
  current_version: null,
  status: "unknown" as const,
  monitoring_interval_seconds: 60,
  failure_threshold: 3,
  latency_threshold_ms: 2000,
  consecutive_failures: 0,
  last_checked_at: null,
  last_healthy_at: null,
  deployment_webhook_configured: false,
  rollback_webhook_configured: false,
  webhook_token_configured: false,
  created_at: "2026-07-25T12:00:00Z",
  updated_at: "2026-07-25T12:00:00Z",
};

describe("NewApplicationPage", () => {
  beforeEach(() => {
    mocks.createApplication.mockReset();
    mocks.push.mockReset();
  });

  it("creates an application and redirects", async () => {
    const user = userEvent.setup();

    mocks.createApplication.mockResolvedValue({
      application: createdApplication,
    });

    render(<NewApplicationPage />);

    await fillRequiredFields(user);

    await user.click(
      screen.getByRole("button", {
        name: "Create application",
      }),
    );

    await waitFor(() => {
      expect(mocks.createApplication).toHaveBeenCalledWith({
        name: "Nimbus API",
        description: "",
        application_url: "https://api.example.com",
        health_url: "https://api.example.com/health",
        repository_url: null,
        environment: "production",
        current_version: null,
        monitoring_interval_seconds: 60,
        failure_threshold: 3,
        latency_threshold_ms: 2000,
        deployment_webhook_url: null,
        rollback_webhook_url: null,
        webhook_token: null,
      });
    });

    expect(mocks.push).toHaveBeenCalledWith("/apps/app-123");
  });

  it("validates the application name", async () => {
    const user = userEvent.setup();

    render(<NewApplicationPage />);

    await user.click(
      screen.getByRole("button", {
        name: "Create application",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Application name must contain at least 2 characters.",
    );

    expect(mocks.createApplication).not.toHaveBeenCalled();
  });

  it("validates application URLs", async () => {
    const user = userEvent.setup();

    render(<NewApplicationPage />);

    await user.type(screen.getByLabelText(/Application name/i), "Nimbus API");

    await user.type(screen.getByLabelText(/Application URL/i), "not-a-url");

    await user.type(
      screen.getByLabelText(/Health URL/i),
      "https://api.example.com/health",
    );

    await user.click(
      screen.getByRole("button", {
        name: "Create application",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Enter a valid application URL.",
    );

    expect(mocks.createApplication).not.toHaveBeenCalled();
  });

  it("disables the button while saving", async () => {
    const user = userEvent.setup();

    let finishCreate: (() => void) | undefined;

    mocks.createApplication.mockImplementation(
      () =>
        new Promise((resolve) => {
          finishCreate = () =>
            resolve({
              application: createdApplication,
            });
        }),
    );

    render(<NewApplicationPage />);

    await fillRequiredFields(user);

    await user.click(
      screen.getByRole("button", {
        name: "Create application",
      }),
    );

    const savingButton = screen.getByRole("button", {
      name: "Saving...",
    });

    expect(savingButton).toBeDisabled();
    expect(savingButton).toHaveAttribute("aria-busy", "true");

    finishCreate?.();

    await waitFor(() => {
      expect(mocks.push).toHaveBeenCalledWith("/apps/app-123");
    });
  });

  it("shows a safe API error", async () => {
    const user = userEvent.setup();

    mocks.createApplication.mockRejectedValue(
      new APIError(
        "An application with this name already exists.",
        409,
        "application_conflict",
        "request-123",
      ),
    );

    render(<NewApplicationPage />);

    await fillRequiredFields(user);

    await user.click(
      screen.getByRole("button", {
        name: "Create application",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "An application with this name already exists.",
    );

    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("does not expose unexpected secrets", async () => {
    const user = userEvent.setup();

    mocks.createApplication.mockRejectedValue(
      new Error(
        "webhook_token=super-secret deployment_url=https://secret.example.com",
      ),
    );

    render(<NewApplicationPage />);

    await fillRequiredFields(user);

    await user.click(
      screen.getByRole("button", {
        name: "Create application",
      }),
    );

    const alert = await screen.findByRole("alert");

    expect(alert).toHaveTextContent("The application could not be saved.");

    expect(alert).not.toHaveTextContent(/super-secret|secret\.example\.com/i);
  });
});

async function fillRequiredFields(user: ReturnType<typeof userEvent.setup>) {
  await user.type(screen.getByLabelText(/Application name/i), "Nimbus API");

  await user.type(
    screen.getByLabelText(/Application URL/i),
    "https://api.example.com",
  );

  await user.type(
    screen.getByLabelText(/Health URL/i),
    "https://api.example.com/health",
  );
}
