import type {
  ApiErrorPayload,
  ApiMetadata,
  DataEnvelope,
  IncidentStatus,
  IncidentSummary,
  ServiceSummary,
  TelemetryWindow,
  TelemetryMetricsResponse,
  TelemetryLogsResponse,
  TelemetryTracesResponse,
  TelemetryServicesResponse,
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

async function getJSON<T>(
  path: string,
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

  let response: Response;
  try {
    response = await fetch(`${apiBaseUrl}${path}`, {
      method: "GET",
      headers: { Accept: "application/json" },
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
): Promise<TelemetryMetricsResponse> {
  const query = new URLSearchParams({ window, service });
  return getJSON<TelemetryMetricsResponse>(
    `/api/v1/telemetry/metrics?${query.toString()}`,
    signal,
    baseUrl,
  );
}

export async function listTelemetryServices(
  window: TelemetryWindow = "1h",
  signal?: AbortSignal,
  baseUrl?: string,
): Promise<TelemetryServicesResponse> {
  return getJSON<TelemetryServicesResponse>(
    `/api/v1/telemetry/services?window=${encodeURIComponent(window)}`,
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
