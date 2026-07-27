import type { Deployment, DeploymentStatus } from "@/types/deployment";

export type IncidentType =
  "health_failure" | "excessive_latency" | "deployment_failure";

export type IncidentSeverity = "warning" | "critical";

export type IncidentStatus = "open" | "acknowledged" | "resolved";

export type Incident = {
  id: string;
  application_id: string;
  incident_type: IncidentType;
  title: string;
  summary: string;
  severity: IncidentSeverity;
  status: IncidentStatus;
  dedup_key: string;

  source_health_check_id: string | null;
  source_deployment_id: string | null;
  rollback_deployment_id: string | null;

  acknowledged_at: string | null;
  acknowledged_by: string | null;

  resolved_at: string | null;
  resolved_by: string | null;

  created_at: string;
  updated_at: string;
};

export type IncidentListItem = Incident & {
  application_name: string;
};

export type IncidentEvent = {
  id: string;
  incident_id: string;
  event_type: string;
  message: string;
  actor_user_id: string | null;
  health_check_id: string | null;
  deployment_id: string | null;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type IncidentListResponse = {
  incidents: IncidentListItem[];
};

export type IncidentDetailResponse = {
  incident: Incident;
  events: IncidentEvent[];
};

export type IncidentActionResponse = {
  incident: Incident;
  event: IncidentEvent | null;
};

export type RollbackResponse = {
  deployment: Deployment;
};

export type LiveIncidentEvent = {
  id: string;
  application_id: string;
  incident_id: string;
  incident_type: IncidentType;
  severity: IncidentSeverity;
  status: IncidentStatus;
  title: string;
  event_type: string;
  message: string;
  actor_user_id: string | null;
  health_check_id: string | null;
  deployment_id: string | null;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type RollbackDeploymentStatus = DeploymentStatus;
