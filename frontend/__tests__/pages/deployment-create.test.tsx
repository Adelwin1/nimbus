import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import NewDeploymentPage from "@/app/apps/[id]/deployments/new/page";
import { APIError } from "@/lib/api";

const mocks = vi.hoisted(() => ({
  createDeployment: vi.fn(),
  push: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/apps/app-123/deployments/new",
  useParams: () => ({
    id: "app-123",
  }),

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
    createDeployment: mocks.createDeployment,
  };
});

describe("NewDeploymentPage", () => {
  beforeEach(() => {
    mocks.createDeployment.mockReset();
    mocks.push.mockReset();
  });

  it("creates a deployment and redirects", async () => {
    const user = userEvent.setup();

    mocks.createDeployment.mockResolvedValue({
      deployment: {
        id: "deployment-123",
      },
    });

    render(<NewDeploymentPage />);

    await user.type(screen.getByLabelText("Version"), "v1.4.0");

    await user.type(screen.getByLabelText("Commit SHA"), "a3c9f24");

    await user.type(
      screen.getByLabelText("Release notes"),
      "Improve monitoring reliability.",
    );

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    await waitFor(() => {
      expect(mocks.createDeployment).toHaveBeenCalledWith("app-123", {
        version: "v1.4.0",
        commit_sha: "a3c9f24",
        release_notes: "Improve monitoring reliability.",
      });
    });

    expect(mocks.push).toHaveBeenCalledWith("/deployments/deployment-123");
  });

  it("requires a version", async () => {
    const user = userEvent.setup();

    render(<NewDeploymentPage />);

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Version is required.",
    );

    expect(mocks.createDeployment).not.toHaveBeenCalled();
  });

  it("validates the commit SHA", async () => {
    const user = userEvent.setup();

    render(<NewDeploymentPage />);

    await user.type(screen.getByLabelText("Version"), "v1.4.0");

    await user.type(screen.getByLabelText("Commit SHA"), "not-a-commit");

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Commit SHA must contain 7 to 64 hexadecimal characters.",
    );

    expect(mocks.createDeployment).not.toHaveBeenCalled();
  });

  it("allows an empty optional commit SHA", async () => {
    const user = userEvent.setup();

    mocks.createDeployment.mockResolvedValue({
      deployment: {
        id: "deployment-456",
      },
    });

    render(<NewDeploymentPage />);

    await user.type(screen.getByLabelText("Version"), "v2.0.0");

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    await waitFor(() => {
      expect(mocks.createDeployment).toHaveBeenCalledWith("app-123", {
        version: "v2.0.0",
        commit_sha: null,
        release_notes: "",
      });
    });
  });

  it("disables submission while creating", async () => {
    const user = userEvent.setup();

    let finishDeployment: (() => void) | undefined;

    mocks.createDeployment.mockImplementation(
      () =>
        new Promise((resolve) => {
          finishDeployment = () =>
            resolve({
              deployment: {
                id: "deployment-789",
              },
            });
        }),
    );

    render(<NewDeploymentPage />);

    await user.type(screen.getByLabelText("Version"), "v3.0.0");

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    const button = screen.getByRole("button", {
      name: "Starting deployment...",
    });

    expect(button).toBeDisabled();
    expect(button).toHaveAttribute("aria-busy", "true");

    finishDeployment?.();

    await waitFor(() => {
      expect(mocks.push).toHaveBeenCalledWith("/deployments/deployment-789");
    });
  });

  it("shows a structured API error", async () => {
    const user = userEvent.setup();

    mocks.createDeployment.mockRejectedValue(
      new APIError(
        "A deployment is already active.",
        409,
        "active_deployment_exists",
        "request-123",
      ),
    );

    render(<NewDeploymentPage />);

    await user.type(screen.getByLabelText("Version"), "v1.5.0");

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "A deployment is already active.",
    );

    expect(mocks.push).not.toHaveBeenCalled();
  });

  it("does not expose unexpected secrets", async () => {
    const user = userEvent.setup();

    mocks.createDeployment.mockRejectedValue(
      new Error("token=super-secret webhook=https://secret.example.com"),
    );

    render(<NewDeploymentPage />);

    await user.type(screen.getByLabelText("Version"), "v1.6.0");

    await user.click(
      screen.getByRole("button", {
        name: "Deploy release",
      }),
    );

    const alert = await screen.findByRole("alert");

    expect(alert).toHaveTextContent("Deployment could not be created.");

    expect(alert).not.toHaveTextContent(/super-secret|secret\.example\.com/i);
  });
});
