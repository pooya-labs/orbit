import { getAccessToken } from "./oidc";
import type {
  AlertInstanceStatus,
  AlertRuleOperator,
  WorkspaceAlertInstance,
  WorkspaceAlertRule,
  WorkspaceAlertNotificationChannel,
  WorkspaceAlertNotificationDelivery,
  WorkspaceAlertNotificationAttempt,
  AlertNotificationDeliveryStatus,
  ApiErrorPayload,
  ApiMetadata,
  DataEnvelope,
  IncidentSeverity,
  IncidentStatus,
  IncidentSummary,
  IncidentTimelineEvent,
  ServiceSummary,
  WorkspaceMembershipSummary,
  WorkspaceAuditEvent,
  WorkspaceSummary,
  WorkspaceSLO,
  WorkspaceSLOBudget,
  WorkspaceDeploymentAnnotation,
  DeploymentAnnotationComparison,
  TelemetryWindow,
  TelemetryMetricsResponse,
  TelemetryLogsResponse,
  TelemetryTracesResponse,
  TelemetryServicesResponse,
  TelemetryResourceAttributesResponse,
  WorkspaceTelemetryServicesResponse,
} from "./contracts";

export class OrbitApiError extends Error {
  readonly status: number;
  readonly code: string;
  readonly requestId: string;

  constructor(message: string, status: number, code: string, requestId = "") {
    super(message);
    this.name = "OrbitApiError";
    this.status = status;
    this.code = code;
    this.requestId = requestId;
  }
}

function resolveApiBaseUrl(override?: string): string {
  return (override ?? process.env.NEXT_PUBLIC_ORBIT_API_URL ?? "")
    .trim()
    .replace(/\/+$/, "");
}

async function requestJSON<T>(
  path: string,
  method: "GET" | "POST" | "PATCH" | "PUT" | "DELETE",
  body?: unknown,
  signal?: AbortSignal,
  baseUrlOverride?: string,
): Promise<T> {
  const apiBaseUrl = resolveApiBaseUrl(baseUrlOverride);
  if (!apiBaseUrl) {
    throw new OrbitApiError(
      "The ORBIT API endpoint is not configured. Enter an API URL or configure NEXT_PUBLIC_ORBIT_API_URL.",
      0,
      "api_not_configured",
    );
  }

  const accessToken = await getAccessToken();
  const headers: HeadersInit = {
    Accept: "application/json",
    ...(body !== undefined ? { "Content-Type": "application/json" } : {}),
    ...(accessToken ? { Authorization: "Bearer " + accessToken } : {}),
  };

  let response: Response;
  try {
    response = await fetch(`${apiBaseUrl}${path}`, {
      method,
      headers,
      ...(body !== undefined ? { body: JSON.stringify(body) } : {}),
      cache: "no-store",
      credentials: "omit",
      signal,
    });
  } catch (error) {
    if (error instanceof DOMException && error.name === "AbortError") {
      throw error;
    }
    throw new OrbitApiError("The ORBIT API could not be reached. Check the URL, network, and CORS allowlist.", 0, "network_error");
  }

  if (response.status === 204) {
    return undefined as T;
  }

  const payload: unknown = await response.json().catch(() => null);
  if (!response.ok) {
    const errorPayload = payload as ApiErrorPayload | null;
    throw new OrbitApiError(
      errorPayload?.error?.message ?? "The ORBIT API returned an unexpected error.",
      response.status,
      errorPayload?.error?.code ?? "http_error",
      errorPayload?.error?.request_id ?? response.headers.get("X-Request-ID") ?? "",
    );
  }

  if (payload === null) {
    throw new OrbitApiError("The ORBIT API returned an empty response.", response.status, "invalid_response");
  }
  return payload as T;
}

async function getJSON<T>(
  path: string,
  signal?: AbortSignal,
  baseUrlOverride?: string,
): Promise<T> {
  return requestJSON<T>(path, "GET", undefined, signal, baseUrlOverride);
}


export async function listWorkspaces(signal?: AbortSignal, baseUrl?: string): Promise<WorkspaceSummary[]> {
  const response = await getJSON<DataEnvelope<WorkspaceSummary>>("/api/v1/workspaces", signal, baseUrl);
  return response.data;
}

export async function getWorkspaceMembership(
  workspaceId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceMembershipSummary> {
  const response = await getJSON<{ data: WorkspaceMembershipSummary }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/membership`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceAuditEvents(
  workspaceId: string,
  limit = 100,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceAuditEvent[]> {
  const acceptedLimit = [10, 25, 50, 100].includes(limit) ? limit : 100;
  const query = new URLSearchParams({ limit: String(acceptedLimit) });
  const response = await getJSON<DataEnvelope<WorkspaceAuditEvent>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/audit-events?${query.toString()}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceServices(
  workspaceId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<ServiceSummary[]> {
  const response = await getJSON<DataEnvelope<ServiceSummary>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/services`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceSLOs(
  workspaceId: string,
  serviceId?: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceSLO[]> {
  const query = new URLSearchParams({ limit: "100" });
  if (serviceId) query.set("service_id", serviceId);
  const response = await getJSON<DataEnvelope<WorkspaceSLO>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/slos?${query.toString()}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export type WorkspaceSLOInput = {
  service_id: string;
  name: string;
  route: string;
  target: number;
  rolling_window_days: 1 | 3 | 7;
};

export async function createWorkspaceSLO(
  workspaceId: string,
  input: WorkspaceSLOInput,
  baseUrl?: string,
): Promise<WorkspaceSLO> {
  const response = await requestJSON<{ data: WorkspaceSLO }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/slos`,
    "POST",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function updateWorkspaceSLO(
  workspaceId: string,
  sloId: string,
  input: Omit<WorkspaceSLOInput, "service_id">,
  baseUrl?: string,
): Promise<WorkspaceSLO> {
  const response = await requestJSON<{ data: WorkspaceSLO }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/slos/${encodeURIComponent(sloId)}`,
    "PUT",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function setWorkspaceSLOEnabled(
  workspaceId: string,
  sloId: string,
  enabled: boolean,
  baseUrl?: string,
): Promise<WorkspaceSLO> {
  const response = await requestJSON<{ data: WorkspaceSLO }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/slos/${encodeURIComponent(sloId)}/enabled`,
    "PATCH",
    { enabled },
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function deleteWorkspaceSLO(
  workspaceId: string,
  sloId: string,
  baseUrl?: string,
): Promise<void> {
  await requestJSON<void>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/slos/${encodeURIComponent(sloId)}`,
    "DELETE",
    undefined,
    undefined,
    baseUrl,
  );
}

export async function getWorkspaceSLOBudget(
  workspaceId: string,
  sloId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceSLOBudget> {
  const response = await getJSON<{ data: WorkspaceSLOBudget }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/slos/${encodeURIComponent(sloId)}/budget`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceDeploymentAnnotations(
  workspaceId: string,
  serviceId?: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceDeploymentAnnotation[]> {
  const query = new URLSearchParams({ limit: "100" });
  if (serviceId) query.set("service_id", serviceId);
  const response = await getJSON<DataEnvelope<WorkspaceDeploymentAnnotation>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/deployments?${query.toString()}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export type WorkspaceDeploymentAnnotationInput = {
  service_id: string;
  version: string;
  commit_sha?: string;
  change_reference?: string;
  release_notes?: string;
  deployed_at?: string;
};

export async function createWorkspaceDeploymentAnnotation(
  workspaceId: string,
  input: WorkspaceDeploymentAnnotationInput,
  baseUrl?: string,
): Promise<WorkspaceDeploymentAnnotation> {
  const response = await requestJSON<{ data: WorkspaceDeploymentAnnotation }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/deployments`,
    "POST",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function compareWorkspaceDeploymentAnnotations(
  workspaceId: string,
  fromId: string,
  toId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<DeploymentAnnotationComparison> {
  const query = new URLSearchParams({ from_id: fromId, to_id: toId });
  return getJSON<DeploymentAnnotationComparison>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/deployments/compare?${query.toString()}`,
    signal,
    baseUrl,
  );
}

export async function listWorkspaceAlertRules(
  workspaceId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceAlertRule[]> {
  const response = await getJSON<DataEnvelope<WorkspaceAlertRule>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-rules?limit=100`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceAlerts(
  workspaceId: string,
  status?: AlertInstanceStatus,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceAlertInstance[]> {
  const query = new URLSearchParams({ limit: "100" });
  if (status) query.set("status", status);
  const response = await getJSON<DataEnvelope<WorkspaceAlertInstance>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alerts?${query.toString()}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export type WorkspaceAlertRuleInput = {
  name: string;
  description: string;
  service_name: string;
  metric: "request_rate";
  operator: AlertRuleOperator;
  threshold: number;
  window_seconds: number;
  pending_seconds: number;
  severity: WorkspaceAlertRule["severity"];
};

export async function createWorkspaceAlertRule(
  workspaceId: string,
  input: WorkspaceAlertRuleInput,
  baseUrl?: string,
): Promise<WorkspaceAlertRule> {
  const response = await requestJSON<{ data: WorkspaceAlertRule }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-rules`,
    "POST",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function updateWorkspaceAlertRule(
  workspaceId: string,
  ruleId: string,
  input: WorkspaceAlertRuleInput,
  baseUrl?: string,
): Promise<WorkspaceAlertRule> {
  const response = await requestJSON<{ data: WorkspaceAlertRule }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-rules/${encodeURIComponent(ruleId)}`,
    "PUT",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function setWorkspaceAlertRuleEnabled(
  workspaceId: string,
  ruleId: string,
  enabled: boolean,
  baseUrl?: string,
): Promise<WorkspaceAlertRule> {
  const response = await requestJSON<{ data: WorkspaceAlertRule }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-rules/${encodeURIComponent(ruleId)}/enabled`,
    "PATCH",
    { enabled },
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function deleteWorkspaceAlertRule(
  workspaceId: string,
  ruleId: string,
  baseUrl?: string,
): Promise<void> {
  await requestJSON<void>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-rules/${encodeURIComponent(ruleId)}`,
    "DELETE",
    undefined,
    undefined,
    baseUrl,
  );
}

export async function listWorkspaceAlertNotificationChannels(
  workspaceId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceAlertNotificationChannel[]> {
  const response = await getJSON<DataEnvelope<WorkspaceAlertNotificationChannel>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-notification-channels?limit=100`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function createWorkspaceAlertNotificationChannel(
  workspaceId: string,
  input: { name: string; secret_ref: string },
  baseUrl?: string,
): Promise<WorkspaceAlertNotificationChannel> {
  const response = await requestJSON<{ data: WorkspaceAlertNotificationChannel }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-notification-channels`,
    "POST",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function setWorkspaceAlertNotificationChannelEnabled(
  workspaceId: string,
  channelId: string,
  enabled: boolean,
  baseUrl?: string,
): Promise<WorkspaceAlertNotificationChannel> {
  const response = await requestJSON<{ data: WorkspaceAlertNotificationChannel }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-notification-channels/${encodeURIComponent(channelId)}/enabled`,
    "PATCH",
    { enabled },
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceAlertNotificationDeliveries(
  workspaceId: string,
  status?: AlertNotificationDeliveryStatus,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceAlertNotificationDelivery[]> {
  const query = new URLSearchParams({ limit: "100" });
  if (status) query.set("status", status);
  const response = await getJSON<DataEnvelope<WorkspaceAlertNotificationDelivery>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-notification-deliveries?${query.toString()}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceAlertNotificationAttempts(
  workspaceId: string,
  deliveryId: string,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceAlertNotificationAttempt[]> {
  const response = await getJSON<DataEnvelope<WorkspaceAlertNotificationAttempt>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/alert-notification-deliveries/${encodeURIComponent(deliveryId)}/attempts?limit=100`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function listWorkspaceIncidents(
  workspaceId: string,
  status?: IncidentStatus,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<IncidentSummary[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  const response = await getJSON<DataEnvelope<IncidentSummary>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/incidents${query}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function createWorkspaceIncident(
  workspaceId: string,
  input: { title: string; description: string; service_id: string; severity: IncidentSeverity },
  baseUrl?: string,
): Promise<IncidentSummary> {
  const response = await requestJSON<{ data: IncidentSummary }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/incidents`,
    "POST",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function transitionWorkspaceIncident(
  workspaceId: string,
  incidentId: string,
  input: { status: IncidentStatus; comment?: string },
  baseUrl?: string,
): Promise<IncidentSummary> {
  const response = await requestJSON<{ data: IncidentSummary }>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/incidents/${encodeURIComponent(incidentId)}/status`,
    "PATCH",
    input,
    undefined,
    baseUrl,
  );
  return response.data;
}

export async function listIncidentTimeline(
  workspaceId: string,
  incidentId: string,
  limit = 50,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<IncidentTimelineEvent[]> {
  const query = new URLSearchParams({ limit: String(limit) });
  const response = await getJSON<DataEnvelope<IncidentTimelineEvent>>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/incidents/${encodeURIComponent(incidentId)}/timeline?${query.toString()}`,
    signal,
    baseUrl,
  );
  return response.data;
}

export async function getMetadata(signal?: AbortSignal, baseUrl?: string): Promise<ApiMetadata> {
  return getJSON<ApiMetadata>("/api/v1/meta", signal, baseUrl);
}

export async function listServices(signal?: AbortSignal, baseUrl?: string): Promise<ServiceSummary[]> {
  const response = await getJSON<DataEnvelope<ServiceSummary>>("/api/v1/services", signal, baseUrl);
  return response.data;
}

export async function listIncidents(
  status?: IncidentStatus,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<IncidentSummary[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  const response = await getJSON<DataEnvelope<IncidentSummary>>(
    `/api/v1/incidents${query}`,
    signal,
    baseUrl,
  );
  return response.data;
}


export async function getTelemetryMetrics(
  window: TelemetryWindow = "1h",
  signal?: AbortSignal,
  baseUrl?: string,
  service = "orbit-telemetry-demo",
  groupBy?: string,
): Promise<TelemetryMetricsResponse> {
  const query = new URLSearchParams({ window, service });
  if (groupBy !== undefined) query.set("group_by", groupBy);
  return getJSON<TelemetryMetricsResponse>(
    `/api/v1/telemetry/metrics?${query.toString()}`,
    signal,
    baseUrl,
  );
}

export async function listWorkspaceTelemetryServices(
  workspaceId: string,
  limit = 50,
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<WorkspaceTelemetryServicesResponse> {
  const acceptedLimit = [10, 25, 50, 100].includes(limit) ? limit : 50;
  const query = new URLSearchParams({ limit: String(acceptedLimit) });
  return getJSON<WorkspaceTelemetryServicesResponse>(
    `/api/v1/workspaces/${encodeURIComponent(workspaceId)}/telemetry/services?${query.toString()}`,
    signal,
    baseUrl,
  );
}

export async function listTelemetryServices(
  window: TelemetryWindow = "1h",
  signal?: AbortSignal,
  baseUrl?: string,
  cursor?: string,
  limit = 50,
): Promise<TelemetryServicesResponse> {
  const query = new URLSearchParams({ window, limit: String(limit) });
  if (cursor) query.set("cursor", cursor);
  return getJSON<TelemetryServicesResponse>(
    `/api/v1/telemetry/services?${query.toString()}`,
    signal,
    baseUrl,
  );
}

export async function getTelemetryServiceAttributes(
  service: string,
  window: TelemetryWindow = "1h",
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<TelemetryResourceAttributesResponse> {
  return getJSON<TelemetryResourceAttributesResponse>(
    `/api/v1/telemetry/services/${encodeURIComponent(service)}/attributes?window=${encodeURIComponent(window)}`,
    signal,
    baseUrl,
  );
}

export async function listTelemetryLogs(
  options: { window?: TelemetryWindow; service?: string; limit?: number } = {},
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<TelemetryLogsResponse> {
  const query = new URLSearchParams();
  query.set("window", options.window ?? "1h");
  query.set("service", options.service ?? "orbit-telemetry-demo");
  if (options.limit !== undefined) query.set("limit", String(options.limit));
  return getJSON<TelemetryLogsResponse>(
    `/api/v1/telemetry/logs?${query.toString()}`,
    signal,
    baseUrl,
  );
}

export async function listTelemetryTraces(
  options: { window?: TelemetryWindow; service?: string; limit?: number } = {},
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<TelemetryTracesResponse> {
  const query = new URLSearchParams();
  query.set("window", options.window ?? "1h");
  query.set("service", options.service ?? "orbit-telemetry-demo");
  if (options.limit !== undefined) query.set("limit", String(options.limit));
  return getJSON<TelemetryTracesResponse>(
    `/api/v1/telemetry/traces?${query.toString()}`,
    signal,
    baseUrl,
  );
}
