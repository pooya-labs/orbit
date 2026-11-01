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


export type TelemetryWindow = "15m" | "1h" | "6h" | "24h";

export interface TelemetryQueryResponse<T> {
  source: "prometheus" | "loki" | "tempo";
  window: TelemetryWindow;
  queried_at: string;
  query?: string;
  step?: string;
  service?: string;
  group_by?: string[];
  limit?: number;
  data: T;
}

export interface PrometheusRangeSeries {
  metric: Record<string, string>;
  values: [number, string][];
}

export interface LokiLogStream {
  stream: Record<string, string>;
  values: [string, string][];
}

export interface TempoTraceSummary {
  traceID: string;
  rootServiceName: string;
  rootTraceName: string;
  startTimeUnixNano: string;
  durationMs: number;
}

export type TelemetryMetricsResponse = TelemetryQueryResponse<PrometheusRangeSeries[]>;
export type TelemetryLogsResponse = TelemetryQueryResponse<LokiLogStream[]>;
export type TelemetryTracesResponse = TelemetryQueryResponse<TempoTraceSummary[]>;


export type TelemetryBackend = "prometheus" | "loki" | "tempo";

export interface DiscoveredTelemetryService {
  id?: string;
  name: string;
  sources: TelemetryBackend[];
  first_seen_at?: string;
  last_seen_at?: string;
  last_discovered_at?: string;
  resource_attributes?: Record<string, unknown>;
  resource_attribute_sources?: Record<string, TelemetryBackend[]>;
}

export interface TelemetryServicesResponse {
  source: "telemetry_backends" | "postgres_catalog";
  window: TelemetryWindow;
  queried_at: string;
  sources_queried: TelemetryBackend[];
  warnings?: string[];
  next_cursor?: string;
  has_more: boolean;
  data: DiscoveredTelemetryService[];
}

export interface TelemetryResourceAttributesResponse {
  service: string;
  window: TelemetryWindow;
  queried_at: string;
  sources_queried: TelemetryBackend[];
  warnings?: string[];
  attributes: Record<string, unknown>;
  attribute_sources: Record<string, TelemetryBackend[]>;
}
