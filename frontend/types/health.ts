import type { ApplicationStatus } from "@/types/application";

export type HealthCheck = {
  id: string;
  application_id: string;
  status_code: number | null;
  latency_ms: number;
  healthy: boolean;
  error_message: string | null;
  checked_at: string;
};

export type ApplicationHealthState = {
  status: ApplicationStatus;
  consecutive_failures: number;
  last_checked_at: string | null;
  last_healthy_at: string | null;
};

export type HealthStatistics = {
  total_checks: number;
  successful_checks: number;
  failed_checks: number;
  availability_percent: number;
  average_latency_ms: number;
  minimum_latency_ms: number;
  maximum_latency_ms: number;
  latest_check: HealthCheck | null;
};

export type HealthOverview = {
  state: ApplicationHealthState;
  statistics: HealthStatistics;
};

export type HealthOverviewResponse = {
  health: HealthOverview;
};

export type HealthHistoryResponse = {
  checks: HealthCheck[];
  total: number;
  limit: number;
  offset: number;
};

export type CheckNowResponse = {
  check: HealthCheck;
  state: ApplicationHealthState;
};
