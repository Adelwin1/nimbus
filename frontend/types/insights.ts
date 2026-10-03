export type ReliabilityOverview = {
  window_start: string;
  generated_at: string;
  metrics: {
    checks: number;
    successful: number;
    success_percent: number | null;
    average_latency_ms: number | null;
    p95_latency_ms: number | null;
  };
  history: {
    time: string;
    checks: number;
    successful: number;
    latency_ms: number | null;
  }[];
  releases: {
    id: string;
    application_name: string;
    version: string;
    previous_version: string | null;
    status: string;
    kind: string;
    created_at: string;
    duration_seconds: number | null;
  }[];
  incidents: {
    id: string;
    application_name: string;
    title: string;
    status: string;
    severity: string;
    created_at: string;
    duration_seconds: number;
  }[];
  active_incidents: number;
};
