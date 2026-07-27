export type ApplicationEnvironment = "development" | "staging" | "production";

export type ApplicationStatus =
  "unknown" | "healthy" | "degraded" | "down" | "deploying";

export type Application = {
  id: string;
  name: string;
  description: string;
  application_url: string;
  health_url: string;
  repository_url: string | null;

  environment: ApplicationEnvironment;
  current_version: string | null;
  status: ApplicationStatus;

  monitoring_interval_seconds: number;
  failure_threshold: number;
  latency_threshold_ms: number;

  consecutive_failures: number;
  last_checked_at: string | null;
  last_healthy_at: string | null;

  deployment_webhook_configured: boolean;
  rollback_webhook_configured: boolean;
  webhook_token_configured: boolean;

  created_at: string;
  updated_at: string;
};

export type CreateApplicationInput = {
  name: string;
  description: string;
  application_url: string;
  health_url: string;
  repository_url?: string | null;

  environment: ApplicationEnvironment;
  current_version?: string | null;

  monitoring_interval_seconds: number;
  failure_threshold: number;
  latency_threshold_ms: number;

  deployment_webhook_url?: string | null;
  rollback_webhook_url?: string | null;
  webhook_token?: string | null;
};

export type UpdateApplicationInput = Partial<CreateApplicationInput>;

export type ApplicationResponse = {
  application: Application;
};

export type ApplicationsResponse = {
  applications: Application[];
};

export type DashboardSummary = {
  total_applications: number;
  healthy_applications: number;
  degraded_applications: number;
  down_applications: number;
  unknown_applications: number;
};

export type DashboardResponse = {
  summary: DashboardSummary;
};
