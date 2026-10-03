import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import type { ReactNode } from "react";
import { beforeEach, describe, expect, it, vi } from "vitest";

import IncidentDetailPage from "@/app/incidents/[id]/page";
import { APIError } from "@/lib/api";

const mocks = vi.hoisted(() => ({
  getIncidentDetail: vi.fn(),
  acknowledgeIncident: vi.fn(),
  resolveIncident: vi.fn(),
  startIncidentRollback: vi.fn(),
  subscribe: vi.fn(),
  push: vi.fn(),
}));

vi.mock("@/components/ui/ToastProvider", () => ({
  useToast: () => ({
    showToast: vi.fn(),
    success: vi.fn(),
    error: vi.fn(),
    info: vi.fn(),
  }),
}));

vi.mock("next/navigation", () => ({
  usePathname: () => "/incidents/incident-123",
  useParams: () => ({
    id: "incident-123",
  }),

  useRouter: () => ({
    push: mocks.push,
  }),
}));

vi.mock("@/components/auth/ProtectedRoute", () => ({
  ProtectedRoute: ({ children }: { children: ReactNode }) => children,
}));

vi.mock("@/lib/live-events", () => ({
  subscribeToApplicationEvents: mocks.subscribe,
}));

vi.mock("@/lib/api", async (importOriginal) => {
  const actual = await importOriginal<typeof import("@/lib/api")>();

  return {
    ...actual,
    getIncidentDetail: mocks.getIncidentDetail,
    acknowledgeIncident: mocks.acknowledgeIncident,
    resolveIncident: mocks.resolveIncident,
    startIncidentRollback: mocks.startIncidentRollback,
  };
});

const baseIncident = {
  id: "incident-123",
  application_id: "app-123",
  incident_type: "health_failure" as const,
  title: "Application health checks failed",
  summary: "Three consecutive health checks failed.",
  severity: "critical" as const,
  status: "open" as const,
  dedup_key: "health_failure",
  source_health_check_id: "check-123",
  source_deployment_id: null,
  rollback_deployment_id: null,
  acknowledged_at: null,
  acknowledged_by: null,
  resolved_at: null,
  resolved_by: null,
  created_at: "2026-07-25T12:00:00Z",
  updated_at: "2026-07-25T12:00:00Z",
};

const openedEvent = {
  id: "event-123",
  incident_id: "incident-123",
  event_type: "incident_opened",
  message: "Incident was opened.",
  actor_user_id: null,
  health_check_id: "check-123",
  deployment_id: null,
  metadata: {},
  created_at: "2026-07-25T12:00:00Z",
};

function makeDetail(status: "open" | "acknowledged" | "resolved") {
  return {
    incident: {
      ...baseIncident,
      status,
      acknowledged_at: status === "open" ? null : "2026-07-25T12:05:00Z",
      resolved_at: status === "resolved" ? "2026-07-25T12:10:00Z" : null,
    },
    events: [openedEvent],
  };
}

describe("IncidentDetailPage", () => {
  beforeEach(() => {
    mocks.getIncidentDetail.mockReset();
    mocks.acknowledgeIncident.mockReset();
    mocks.resolveIncident.mockReset();
    mocks.startIncidentRollback.mockReset();
    mocks.subscribe.mockReset();
    mocks.push.mockReset();

    mocks.subscribe.mockReturnValue(vi.fn());
  });

  it("shows a loading state", () => {
    mocks.getIncidentDetail.mockReturnValue(new Promise(() => {}));

    render(<IncidentDetailPage />);

    expect(screen.getByText("Loading incident...")).toBeInTheDocument();
  });

  it("renders the incident and timeline", async () => {
    mocks.getIncidentDetail.mockResolvedValue(makeDetail("open"));

    render(<IncidentDetailPage />);

    expect(
      await screen.findByRole("heading", {
        name: baseIncident.title,
      }),
    ).toBeInTheDocument();

    expect(screen.getByText("Incident was opened.")).toBeInTheDocument();

    expect(screen.getByText("open")).toBeInTheDocument();
  });

  it("acknowledges an incident", async () => {
    const user = userEvent.setup();

    mocks.getIncidentDetail
      .mockResolvedValueOnce(makeDetail("open"))
      .mockResolvedValueOnce(makeDetail("acknowledged"));

    mocks.acknowledgeIncident.mockResolvedValue({
      incident: makeDetail("acknowledged").incident,
      event: openedEvent,
    });

    render(<IncidentDetailPage />);

    await user.click(
      await screen.findByRole("button", {
        name: "Acknowledge",
      }),
    );

    await waitFor(() => {
      expect(mocks.acknowledgeIncident).toHaveBeenCalledWith("incident-123");
    });

    expect(await screen.findByText("acknowledged")).toBeInTheDocument();
  });

  it("resolves an incident", async () => {
    const user = userEvent.setup();

    mocks.getIncidentDetail
      .mockResolvedValueOnce(makeDetail("acknowledged"))
      .mockResolvedValueOnce(makeDetail("resolved"));

    mocks.resolveIncident.mockResolvedValue({
      incident: makeDetail("resolved").incident,
      event: openedEvent,
    });

    render(<IncidentDetailPage />);

    await user.click(
      await screen.findByRole("button", {
        name: "Resolve",
      }),
    );

    await user.click(
      await screen.findByRole("button", {
        name: "Resolve incident",
      }),
    );

    await waitFor(() => {
      expect(mocks.resolveIncident).toHaveBeenCalledWith("incident-123");
    });

    expect(await screen.findByText("resolved")).toBeInTheDocument();
  });

  it("confirms and starts a rollback", async () => {
    const user = userEvent.setup();

    mocks.getIncidentDetail.mockResolvedValue(makeDetail("open"));

    mocks.startIncidentRollback.mockResolvedValue({
      deployment: {
        id: "rollback-123",
      },
    });

    render(<IncidentDetailPage />);

    await user.click(
      await screen.findByRole("button", {
        name: "Start rollback",
      }),
    );

    const dialog = screen.getByRole("dialog");

    expect(
      within(dialog).getByText("Start verified rollback?"),
    ).toBeInTheDocument();

    await user.click(
      within(dialog).getByRole("button", {
        name: "Start rollback",
      }),
    );

    await waitFor(() => {
      expect(mocks.startIncidentRollback).toHaveBeenCalledWith("incident-123");
    });

    expect(mocks.push).toHaveBeenCalledWith("/deployments/rollback-123");
  });

  it("shows safe error messages", async () => {
    mocks.getIncidentDetail.mockRejectedValue(
      new APIError(
        "Incident was not found.",
        404,
        "incident_not_found",
        "request-123",
      ),
    );

    render(<IncidentDetailPage />);

    expect(
      await screen.findByText("Incident was not found."),
    ).toBeInTheDocument();
  });

  it("does not expose unexpected secrets", async () => {
    mocks.getIncidentDetail.mockRejectedValue(
      new Error("token=super-secret webhook=https://secret.example.com"),
    );

    render(<IncidentDetailPage />);

    expect(
      await screen.findByText("Incident could not be loaded."),
    ).toBeInTheDocument();

    expect(
      screen.queryByText(/super-secret|secret\.example\.com/i),
    ).not.toBeInTheDocument();
  });
});
