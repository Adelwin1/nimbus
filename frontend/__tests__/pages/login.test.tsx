import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { beforeEach, describe, expect, it, vi } from "vitest";

import LoginPage from "@/app/login/page";
import { APIError } from "@/lib/api";

const mocks = vi.hoisted(() => ({
  login: vi.fn(),
  replace: vi.fn(),
}));

vi.mock("next/navigation", () => ({
  useRouter: () => ({
    replace: mocks.replace,
  }),
}));

vi.mock("@/contexts/AuthContext", () => ({
  useAuth: () => ({
    login: mocks.login,
  }),
}));

describe("LoginPage", () => {
  beforeEach(() => {
    mocks.login.mockReset();
    mocks.replace.mockReset();
  });

  it("logs in and redirects to the dashboard", async () => {
    const user = userEvent.setup();

    mocks.login.mockResolvedValue(undefined);

    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email"), "adel@example.com");

    await user.type(screen.getByLabelText("Password"), "StrongPassword123!");

    await user.click(
      screen.getByRole("button", {
        name: "Sign in",
      }),
    );

    await waitFor(() => {
      expect(mocks.login).toHaveBeenCalledWith({
        email: "adel@example.com",
        password: "StrongPassword123!",
      });
    });

    expect(mocks.replace).toHaveBeenCalledWith("/dashboard");
  });

  it("shows an invalid credentials error", async () => {
    const user = userEvent.setup();

    mocks.login.mockRejectedValue(
      new APIError(
        "Invalid email or password.",
        401,
        "invalid_credentials",
        "request-123",
      ),
    );

    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email"), "adel@example.com");

    await user.type(screen.getByLabelText("Password"), "WrongPassword123!");

    await user.click(
      screen.getByRole("button", {
        name: "Sign in",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Invalid email or password.",
    );

    expect(mocks.replace).not.toHaveBeenCalled();
  });

  it("shows loading while login is pending", async () => {
    const user = userEvent.setup();

    let finishLogin: (() => void) | undefined;

    mocks.login.mockImplementation(
      () =>
        new Promise<void>((resolve) => {
          finishLogin = resolve;
        }),
    );

    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email"), "adel@example.com");

    await user.type(screen.getByLabelText("Password"), "StrongPassword123!");

    await user.click(
      screen.getByRole("button", {
        name: "Sign in",
      }),
    );

    const button = screen.getByRole("button", {
      name: "Signing in...",
    });

    expect(button).toBeDisabled();

    finishLogin?.();

    await waitFor(() => {
      expect(mocks.replace).toHaveBeenCalledWith("/dashboard");
    });
  });

  it("validates invalid form values", async () => {
    const user = userEvent.setup();

    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email"), "not-an-email");

    await user.click(
      screen.getByRole("button", {
        name: "Sign in",
      }),
    );

    expect(await screen.findByText(/invalid email/i)).toBeInTheDocument();

    expect(screen.getByText("Password is required.")).toBeInTheDocument();

    expect(mocks.login).not.toHaveBeenCalled();
  });

  it("does not expose unexpected error secrets", async () => {
    const user = userEvent.setup();

    mocks.login.mockRejectedValue(
      new Error("password=super-secret token=secret-token"),
    );

    render(<LoginPage />);

    await user.type(screen.getByLabelText("Email"), "adel@example.com");

    await user.type(screen.getByLabelText("Password"), "StrongPassword123!");

    await user.click(
      screen.getByRole("button", {
        name: "Sign in",
      }),
    );

    expect(await screen.findByRole("alert")).toHaveTextContent(
      "Nimbus could not sign you in.",
    );

    expect(
      screen.queryByText(/super-secret|secret-token/i),
    ).not.toBeInTheDocument();
  });
});
