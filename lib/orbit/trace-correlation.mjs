const TRACE_ID_PATTERN = /^[0-9a-f]{32}$/i;
const ZERO_TRACE_ID = /^0{32}$/;

function normalizeTraceId(value) {
  if (typeof value !== "string") return null;
  const candidate = value.trim().toLowerCase();
  if (!TRACE_ID_PATTERN.test(candidate) || ZERO_TRACE_ID.test(candidate)) return null;
  return candidate;
}

function normalizedKey(value) {
  return value.toLowerCase().replace(/[^a-z0-9]/g, "");
}

function fromStructuredValue(value, depth = 0) {
  if (!value || typeof value !== "object" || Array.isArray(value)) return null;

  for (const [key, candidate] of Object.entries(value)) {
    if (normalizedKey(key) === "traceid") {
      const traceId = normalizeTraceId(candidate);
      if (traceId) return traceId;
    }
  }

  if (depth >= 2) return null;
  for (const [key, candidate] of Object.entries(value)) {
    if (!["attributes", "resource", "context", "metadata"].includes(normalizedKey(key))) continue;
    const traceId = fromStructuredValue(candidate, depth + 1);
    if (traceId) return traceId;
  }
  return null;
}

/**
 * Extract a W3C/OpenTelemetry trace ID from a Loki line or its stream labels.
 * Trace IDs are parsed as data only; this never changes a Loki query or label set.
 */
export function extractTraceIdFromLog(line, labels = {}) {
  const labelTraceId = fromStructuredValue(labels);
  if (labelTraceId) return labelTraceId;

  const text = typeof line === "string" ? line : "";
  if (!text) return null;

  try {
    const structuredTraceId = fromStructuredValue(JSON.parse(text));
    if (structuredTraceId) return structuredTraceId;
  } catch {
    // Plain-text log lines are expected and use the bounded fallback below.
  }

  const match = /(?:^|[^\w])trace[._-]?id\s*[:=]\s*["']?([0-9a-f]{32})(?![0-9a-z])/i.exec(text);
  return normalizeTraceId(match?.[1]);
}
