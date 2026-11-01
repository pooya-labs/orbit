import type {
  ApiErrorPayload,
  ApiMetadata,
  DataEnvelope,
  IncidentStatus,
  IncidentSummary,
  ServiceSummary,
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

const apiBaseUrl = (process.env.NEXT_PUBLIC_ORBIT_API_URL ?? "")
  .trim()
  .replace(/\/+$/, "");

async function getJSON<T>(path: string, signal?: AbortSignal): Promise<T> {
  if (!apiBaseUrl) {
    throw new OrbitApiError(
      "The ORBIT API endpoint is not configured for this build.",
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
    throw new OrbitApiError("The ORBIT API could not be reached.", 0, "network_error");
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

export async function getMetadata(signal?: AbortSignal): Promise<ApiMetadata> {
  return getJSON<ApiMetadata>("/api/v1/meta", signal);
}

export async function listServices(signal?: AbortSignal): Promise<ServiceSummary[]> {
  const response = await getJSON<DataEnvelope<ServiceSummary>>("/api/v1/services", signal);
  return response.data;
}

export async function listIncidents(
  status?: IncidentStatus,
  signal?: AbortSignal,
): Promise<IncidentSummary[]> {
  const query = status ? `?status=${encodeURIComponent(status)}` : "";
  const response = await getJSON<DataEnvelope<IncidentSummary>>(
    `/api/v1/incidents${query}`,
    signal,
  );
  return response.data;
}
