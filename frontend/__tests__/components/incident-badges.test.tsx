import { render, screen } from "@testing-library/react";
import { describe, expect, it } from "vitest";

import {
  IncidentSeverityBadge,
  IncidentStatusBadge,
} from "@/components/incident/IncidentBadges";

describe("Incident badges", () => {
  it("renders an open status", () => {
    render(<IncidentStatusBadge status="open" />);

    expect(screen.getByText("open")).toBeInTheDocument();
  });

  it("renders an acknowledged status", () => {
    render(<IncidentStatusBadge status="acknowledged" />);

    expect(screen.getByText("acknowledged")).toBeInTheDocument();
  });

  it("renders a critical severity", () => {
    render(<IncidentSeverityBadge severity="critical" />);

    expect(screen.getByText("critical")).toBeInTheDocument();
  });

  it("renders a warning severity", () => {
    render(<IncidentSeverityBadge severity="warning" />);

    expect(screen.getByText("warning")).toBeInTheDocument();
  });
});
