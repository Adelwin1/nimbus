export type DeploymentStatus =
  | "pending"
  | "triggering"
  | "verifying"
  | "successful"
  | "failed"
  | "cancelled";

export type DeploymentType = "deployment" | "rollback";

export type Deployment = {
  id: string;
  application_id: string;
  version: string;
  commit_sha: string | null;
  release_notes: string;
  deployment_type: DeploymentType;
  status: DeploymentStatus;
  previous_version: string | null;
  started_at: string | null;
  completed_at: string | null;
  created_at: string;
  updated_at: string;
};

export type DeploymentEvent = {
  id: string;
  deployment_id: string;
  event_type: string;
  message: string;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type CreateDeploymentInput = {
  version: string;
  commit_sha?: string | null;
  release_notes: string;
};

export type CreateDeploymentResponse = {
  deployment: Deployment;
};

export type DeploymentListResponse = {
  deployments: Deployment[];
};

export type DeploymentDetailResponse = {
  deployment: Deployment;
  events: DeploymentEvent[];
};

export type LiveConnectedEvent = {
  application_id: string;
  connected_at: string;
};

export type LiveHealthEvent = {
  id: string;
  application_id: string;
  status_code: number | null;
  latency_ms: number;
  healthy: boolean;
  error_message: string | null;
  checked_at: string;
};

export type LiveDeploymentEvent = {
  id: string;
  application_id: string;
  deployment_id: string;
  version: string;
  deployment_type: DeploymentType;
  status: DeploymentStatus;
  event_type: string;
  message: string;
  metadata: Record<string, unknown>;
  created_at: string;
};

export type LiveStreamError = {
  message: string;
};
