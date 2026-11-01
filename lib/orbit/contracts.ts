export type ServiceStatus = "operational" | "degraded" | "incident" | "unknown";
export type IncidentSeverity = "critical" | "high" | "medium" | "low";
export type IncidentStatus = "open" | "investigating" | "monitoring" | "resolved";
export type DataSource = "demo_seed" | "live";

export interface ServiceSummary {
  id: string;
  name: string;
  description: string;
  environment: string;
  region: string;
  status: ServiceStatus;
  latency_p95_ms: number | null;
  /** Decimal fraction: 0.01 represents one percent. */
  error_rate: number | null;
  last_seen_at: string | null;
  source: DataSource;
}

export interface IncidentSummary {
  id: string;
  title: string;
  description: string;
  service_id: string;
  service_name: string;
  severity: IncidentSeverity;
  status: IncidentStatus;
  started_at: string;
  updated_at: string;
  resolved_at: string | null;
  source: DataSource;
}

export interface ApiMetadata {
  name: "orbit-api";
  version: string;
  environment: string;
}

export interface ApiErrorPayload {
  error: {
    code: string;
    message: string;
    request_id: string;
  };
}

export interface DataEnvelope<T> {
  data: T[];
}
