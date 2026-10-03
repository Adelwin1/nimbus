import type {
  IncidentActionResponse,
  IncidentDetailResponse,
  IncidentListResponse,
  RollbackResponse,
} from "@/types/incident";
import type {
  CheckNowResponse,
  HealthHistoryResponse,
  HealthOverviewResponse,
} from "@/types/health";

import type {
  ApplicationResponse,
  ApplicationsResponse,
  CreateApplicationInput,
  DashboardResponse,
  UpdateApplicationInput,
} from "@/types/application";

import type {
  APIErrorResponse,
  AuthResponse,
  CurrentUserResponse,
} from "@/types/auth";

import {
  clearTokens,
  getAccessToken,
  getRefreshToken,
  saveTokens,
} from "@/lib/token-storage";
import type {
  CreateDeploymentInput,
  CreateDeploymentResponse,
  DeploymentDetailResponse,
  DeploymentListResponse,
} from "@/types/deployment";

const API_URL =
  process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080/api/v1";

export class APIError extends Error {
  status: number;
  code?: string;
  requestId?: string;

  constructor(
    message: string,
    status: number,
    code?: string,
    requestId?: string,
  ) {
    super(message);

    this.name = "APIError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

async function parseError(response: Response): Promise<APIError> {
  let payload: APIErrorResponse | null = null;

  try {
    payload = (await response.json()) as APIErrorResponse;
  } catch {
    payload = null;
  }

  return new APIError(
    payload?.error?.message ?? "The request could not be completed.",
    response.status,
    payload?.error?.code,
    payload?.error?.request_id,
  );
}

async function refreshAccessToken(): Promise<string | null> {
  const refreshToken = getRefreshToken();

  if (!refreshToken) {
    return null;
  }

  const response = await fetch(`${API_URL}/auth/refresh`, {
    method: "POST",
    headers: {
      "Content-Type": "application/json",
    },
    body: JSON.stringify({
      refresh_token: refreshToken,
    }),
  });

  if (!response.ok) {
    clearTokens();
    return null;
  }

  const data = (await response.json()) as AuthResponse;

  saveTokens(data.access_token, data.refresh_token);

  return data.access_token;
}

type APIRequestOptions = RequestInit & {
  authenticated?: boolean;
  retryOnUnauthorized?: boolean;
};

export async function apiRequest<T>(
  path: string,
  options: APIRequestOptions = {},
): Promise<T> {
  const {
    authenticated = false,
    retryOnUnauthorized = true,
    headers,
    ...requestOptions
  } = options;

  const requestHeaders = new Headers(headers);

  if (
    requestOptions.body !== undefined &&
    !requestHeaders.has("Content-Type")
  ) {
    requestHeaders.set("Content-Type", "application/json");
  }

  if (authenticated) {
    const accessToken = getAccessToken();

    if (accessToken) {
      requestHeaders.set("Authorization", `Bearer ${accessToken}`);
    }
  }

  let response = await fetch(`${API_URL}${path}`, {
    ...requestOptions,
    headers: requestHeaders,
  });

  if (authenticated && response.status === 401 && retryOnUnauthorized) {
    const newAccessToken = await refreshAccessToken();

    if (newAccessToken) {
      requestHeaders.set("Authorization", `Bearer ${newAccessToken}`);

      response = await fetch(`${API_URL}${path}`, {
        ...requestOptions,
        headers: requestHeaders,
      });
    }
  }

  if (!response.ok) {
    throw await parseError(response);
  }

  if (response.status === 204) {
    return undefined as T;
  }

  return (await response.json()) as T;
}

export function registerUser(input: {
  name: string;
  email: string;
  password: string;
}): Promise<AuthResponse> {
  return apiRequest<AuthResponse>("/auth/register", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function loginUser(input: {
  email: string;
  password: string;
}): Promise<AuthResponse> {
  return apiRequest<AuthResponse>("/auth/login", {
    method: "POST",
    body: JSON.stringify(input),
  });
}

export function getCurrentUser(): Promise<CurrentUserResponse> {
  return apiRequest<CurrentUserResponse>("/auth/me", {
    method: "GET",
    authenticated: true,
  });
}

export async function logoutUser(): Promise<void> {
  const refreshToken = getRefreshToken();

  try {
    if (refreshToken) {
      await apiRequest<void>("/auth/logout", {
        method: "POST",
        body: JSON.stringify({
          refresh_token: refreshToken,
        }),
      });
    }
  } finally {
    clearTokens();
  }
}
export function createApplication(
  input: CreateApplicationInput,
): Promise<ApplicationResponse> {
  return apiRequest<ApplicationResponse>("/apps", {
    method: "POST",
    authenticated: true,
    body: JSON.stringify(input),
  });
}

export function listApplications(): Promise<ApplicationsResponse> {
  return apiRequest<ApplicationsResponse>("/apps", {
    method: "GET",
    authenticated: true,
  });
}

export function getApplication(
  applicationId: string,
): Promise<ApplicationResponse> {
  return apiRequest<ApplicationResponse>(`/apps/${applicationId}`, {
    method: "GET",
    authenticated: true,
  });
}

export function updateApplication(
  applicationId: string,
  input: UpdateApplicationInput,
): Promise<ApplicationResponse> {
  return apiRequest<ApplicationResponse>(`/apps/${applicationId}`, {
    method: "PATCH",
    authenticated: true,
    body: JSON.stringify(input),
  });
}

export function deleteApplication(applicationId: string): Promise<void> {
  return apiRequest<void>(`/apps/${applicationId}`, {
    method: "DELETE",
    authenticated: true,
  });
}

export function getDashboardSummary(): Promise<DashboardResponse> {
  return apiRequest<DashboardResponse>("/dashboard", {
    method: "GET",
    authenticated: true,
  });
}

export function getHealthOverview(
  applicationId: string,
): Promise<HealthOverviewResponse> {
  return apiRequest<HealthOverviewResponse>(`/apps/${applicationId}/health`, {
    method: "GET",
    authenticated: true,
  });
}

export function getHealthHistory(
  applicationId: string,
  limit = 50,
  offset = 0,
): Promise<HealthHistoryResponse> {
  const query = new URLSearchParams({
    limit: String(limit),
    offset: String(offset),
  });

  return apiRequest<HealthHistoryResponse>(
    `/apps/${applicationId}/health/history?${query.toString()}`,
    {
      method: "GET",
      authenticated: true,
    },
  );
}

export function runHealthCheck(
  applicationId: string,
): Promise<CheckNowResponse> {
  return apiRequest<CheckNowResponse>(`/apps/${applicationId}/health/check`, {
    method: "POST",
    authenticated: true,
  });
}

export async function createDeployment(
  applicationId: string,
  input: CreateDeploymentInput,
): Promise<CreateDeploymentResponse> {
  return apiRequest<CreateDeploymentResponse>(
    `/apps/${applicationId}/deployments`,
    {
      method: "POST",
      authenticated: true,
      body: JSON.stringify(input),
    },
  );
}

export async function getDeployments(
  applicationId: string,
): Promise<DeploymentListResponse> {
  return apiRequest<DeploymentListResponse>(
    `/apps/${applicationId}/deployments`,
    {
      method: "GET",
      authenticated: true,
    },
  );
}

export async function getDeploymentDetail(
  deploymentId: string,
): Promise<DeploymentDetailResponse> {
  return apiRequest<DeploymentDetailResponse>(`/deployments/${deploymentId}`, {
    method: "GET",
    authenticated: true,
  });
}

export async function getIncidents(): Promise<IncidentListResponse> {
  return apiRequest<IncidentListResponse>("/incidents", {
    method: "GET",
    authenticated: true,
  });
}

export async function getApplicationIncidents(
  applicationId: string,
): Promise<IncidentListResponse> {
  return apiRequest<IncidentListResponse>(`/apps/${applicationId}/incidents`, {
    method: "GET",
    authenticated: true,
  });
}

export async function getIncidentDetail(
  incidentId: string,
): Promise<IncidentDetailResponse> {
  return apiRequest<IncidentDetailResponse>(`/incidents/${incidentId}`, {
    method: "GET",
    authenticated: true,
  });
}

export async function acknowledgeIncident(
  incidentId: string,
): Promise<IncidentActionResponse> {
  return apiRequest<IncidentActionResponse>(
    `/incidents/${incidentId}/acknowledge`,
    {
      method: "POST",
      authenticated: true,
    },
  );
}

export async function resolveIncident(
  incidentId: string,
): Promise<IncidentActionResponse> {
  return apiRequest<IncidentActionResponse>(
    `/incidents/${incidentId}/resolve`,
    {
      method: "POST",
      authenticated: true,
    },
  );
}

export async function startIncidentRollback(
  incidentId: string,
): Promise<RollbackResponse> {
  return apiRequest<RollbackResponse>(`/incidents/${incidentId}/rollback`, {
    method: "POST",
    authenticated: true,
  });
}

export function getReliabilityOverview(): Promise<import("@/types/insights").ReliabilityOverview> {
  return apiRequest("/dashboard/analytics", { method: "GET", authenticated: true });
}
