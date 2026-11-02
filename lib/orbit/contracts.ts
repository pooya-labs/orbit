export type ServiceStatus = "operational" | "degraded" | "incident" | "unknown";
export type IncidentSeverity = "critical" | "high" | "medium" | "low";
export type IncidentStatus = "open" | "investigating" | "monitoring" | "resolved";

export interface WorkspaceSummary {
  id: string;
  name: string;
  slug: string;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceMembershipSummary {
  workspace_id: string;
  role: "owner" | "administrator" | "operator" | "viewer" | string;
  created_at: string;
  updated_at: string;
}

export interface IncidentTimelineEvent {
  id: string;
  workspace_id: string;
  incident_id: string;
  event_type: string;
  from_status?: IncidentStatus;
  to_status?: IncidentStatus;
  comment?: string;
  request_id?: string;
  details?: Record<string, unknown>;
  created_at: string;
}

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

export type AlertRuleOperator = "gt" | "gte" | "lt" | "lte";
export type AlertInstanceStatus = "pending" | "firing" | "resolved" | "cancelled";

export interface WorkspaceAlertRule {
  id: string;
  workspace_id: string;
  name: string;
  description: string;
  service_name?: string;
  metric: "request_rate";
  operator: AlertRuleOperator;
  threshold: number;
  window_seconds: number;
  pending_seconds: number;
  severity: IncidentSeverity;
  enabled: boolean;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceAlertInstance {
  id: string;
  workspace_id: string;
  alert_rule_id: string;
  rule_name: string;
  service_name: string;
  metric: "request_rate";
  operator: AlertRuleOperator;
  threshold: number;
  window_seconds: number;
  pending_seconds: number;
  severity: IncidentSeverity;
  status: AlertInstanceStatus;
  value: number;
  pending_since: string;
  fired_at: string | null;
  resolved_at: string | null;
  last_evaluated_at: string;
  created_at: string;
  updated_at: string;
}

export type AlertNotificationDeliveryStatus = "pending" | "processing" | "delivered" | "dead";
export type AlertNotificationAttemptOutcome = "delivered" | "retry_scheduled" | "dead";

export interface WorkspaceAlertNotificationChannel {
  id: string;
  workspace_id: string;
  name: string;
  adapter: "webhook";
  enabled: boolean;
  secret_configured: boolean;
  created_at: string;
  updated_at: string;
}

export interface WorkspaceAlertNotificationDelivery {
  id: string;
  workspace_id: string;
  alert_instance_id: string;
  event_id: string;
  channel_id: string;
  channel_name: string;
  event_type: "alert.firing" | "alert.resolved";
  status: AlertNotificationDeliveryStatus;
  attempts: number;
  next_attempt_at: string;
  last_attempt_at?: string | null;
  delivered_at?: string | null;
  last_error: string;
  created_at: string;
  updated_at: string;
  rule_name: string;
  service_name: string;
  metric: "request_rate";
  operator: AlertRuleOperator;
  threshold: number;
  value: number;
  severity: IncidentSeverity;
  instance_status: AlertInstanceStatus;
  occurred_at: string;
}

export interface WorkspaceAlertNotificationAttempt {
  id: string;
  delivery_id: string;
  attempt_number: number;
  outcome: AlertNotificationAttemptOutcome;
  http_status?: number | null;
  error_code?: string;
  attempted_at: string;
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
