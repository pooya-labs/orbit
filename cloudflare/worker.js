const API_PREFIX = "/api/v1";
const MAX_BODY_BYTES = 1024 * 1024;
const MAX_QUERY_ROWS = 250;
const WINDOWS = new Set(["15m", "1h", "6h", "24h"]);
const WINDOW_MS = { "15m": 900000, "1h": 3600000, "6h": 21600000, "24h": 86400000 };

function json(data, status = 200, extra = {}) {
  return new Response(JSON.stringify(data), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store", ...extra }
  });
}
function fail(status, code, message, requestId) {
  return json({ error: { code, message, request_id: requestId } }, status);
}
function requestId() { return crypto.randomUUID(); }
function cors(response, env) {
  const headers = new Headers(response.headers);
  headers.set("access-control-allow-origin", env.ALLOWED_ORIGIN || "https://pooya-labs.github.io");
  headers.set("access-control-allow-methods", "GET,POST,PUT,PATCH,DELETE,OPTIONS");
  headers.set("access-control-allow-headers", "authorization,content-type");
  headers.set("vary", "origin");
  return new Response(response.body, { status: response.status, statusText: response.statusText, headers });
}
let jwksCache = { issuer: "", expiresAt: 0, document: null };
async function readBoundedBody(request, maxBytes) {
  const declared = Number(request.headers.get("content-length") || 0);
  if (declared > maxBytes) throw Object.assign(new Error("OTLP request exceeds 1 MiB"), { status: 413 });
  if (!request.body) return "";
  const reader = request.body.getReader();
  const chunks = [];
  let total = 0;
  while (true) {
    const { done, value } = await reader.read();
    if (done) break;
    total += value.byteLength;
    if (total > maxBytes) { await reader.cancel(); throw Object.assign(new Error("OTLP request exceeds 1 MiB"), { status: 413 }); }
    chunks.push(value);
  }
  const joined = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) { joined.set(chunk, offset); offset += chunk.byteLength; }
  return new TextDecoder().decode(joined);
}
function bearer(request) {
  const value = request.headers.get("authorization") || "";
  return value.startsWith("Bearer ") ? value.slice(7) : "";
}
function b64url(value) {
  const normalized = value.replace(/-/g, "+").replace(/_/g, "/");
  return atob(normalized + "=".repeat((4 - normalized.length % 4) % 4));
}
function decodePart(value) { return JSON.parse(b64url(value)); }
function bytes(value) { return new TextEncoder().encode(value); }
function equal(a, b) {
  if (a.length !== b.length) return false;
  let diff = 0;
  for (let i = 0; i < a.length; i++) diff |= a.charCodeAt(i) ^ b.charCodeAt(i);
  return diff === 0;
}
async function jwks(env, issuer) {
  if (jwksCache.issuer === issuer && jwksCache.expiresAt > Date.now() && jwksCache.document) return jwksCache.document;
  const discoveryUrl = issuer.replace(/\/$/, "") + "/.well-known/openid-configuration";
  const discovery = await fetch(discoveryUrl, { headers: { accept: "application/json" } });
  if (!discovery.ok) throw new Error("OIDC discovery unavailable");
  const config = await discovery.json();
  if (config.issuer !== issuer || typeof config.jwks_uri !== "string") throw new Error("OIDC issuer mismatch");
  const response = await fetch(config.jwks_uri, { headers: { accept: "application/json" } });
  if (!response.ok) throw new Error("OIDC signing keys unavailable");
  const document = await response.json();
  jwksCache = { issuer, expiresAt: Date.now() + 5 * 60 * 1000, document };
  return document;
}
async function verifyJwt(token, env, expectedAudience = env.OIDC_AUDIENCE) {
  if (!env.OIDC_ISSUER || !env.OIDC_AUDIENCE) throw new Error("OIDC is not configured");
  if (token.length > 16384) throw new Error("Bearer token is too large");
  const parts = token.split(".");
  if (parts.length !== 3) throw new Error("Invalid bearer token");
  const header = decodePart(parts[0]);
  const claims = decodePart(parts[1]);
  if (!["RS256", "ES256"].includes(header.alg) || !header.kid) throw new Error("Unsupported token algorithm");
  const keys = await jwks(env, env.OIDC_ISSUER);
  const jwk = (keys.keys || []).find(k => k.kid === header.kid && (!k.alg || k.alg === header.alg));
  if (!jwk) throw new Error("Unknown signing key");
  const algorithm = header.alg === "RS256"
    ? { name: "RSASSA-PKCS1-v1_5", hash: "SHA-256" }
    : { name: "ECDSA", namedCurve: "P-256", hash: "SHA-256" };
  const key = await crypto.subtle.importKey("jwk", jwk, algorithm, false, ["verify"]);
  const signature = Uint8Array.from(b64url(parts[2]), c => c.charCodeAt(0));
  const valid = await crypto.subtle.verify(
    header.alg === "RS256" ? "RSASSA-PKCS1-v1_5" : { name: "ECDSA", hash: "SHA-256" },
    key, signature, bytes(parts[0] + "." + parts[1])
  );
  const now = Math.floor(Date.now() / 1000);
  const audience = Array.isArray(claims.aud) ? claims.aud : [claims.aud];
  if (!valid || claims.iss !== env.OIDC_ISSUER || !audience.includes(expectedAudience) ||
      !claims.sub || !Number.isFinite(claims.exp) || claims.exp <= now ||
      (claims.nbf && claims.nbf > now) || (claims.iat && claims.iat > now + 60)) {
    throw new Error("Bearer token validation failed");
  }
  return claims;
}
async function requireMember(request, env, workspaceId) {
  const token = bearer(request);
  if (!token) throw Object.assign(new Error("Authentication required"), { status: 401 });
  let claims;
  try { claims = await verifyJwt(token, env); }
  catch { throw Object.assign(new Error("A valid ORBIT identity token is required"), { status: 401 }); }
  const membership = await env.DB.prepare(
    "SELECT p.id AS principal_id, m.role FROM principals p JOIN memberships m ON m.principal_id=p.id JOIN workspaces w ON w.id=m.workspace_id WHERE p.issuer=? AND p.subject=? AND w.id=?"
  ).bind(claims.iss, claims.sub, workspaceId).first();
  if (!membership) throw Object.assign(new Error("Workspace access denied"), { status: 403 });
  return { ...claims, principalId: membership.principal_id, role: membership.role };
}
function parseWindow(url) {
  const value = url.searchParams.get("window") || "1h";
  if (!WINDOWS.has(value)) throw Object.assign(new Error("Unsupported time window"), { status: 400 });
  return { value, since: Date.now() - WINDOW_MS[value] };
}
function safeLimit(url) {
  const n = Number(url.searchParams.get("limit") || 100);
  if (!Number.isInteger(n) || n < 1 || n > MAX_QUERY_ROWS) throw Object.assign(new Error("Limit must be between 1 and 250"), { status: 400 });
  return n;
}
function toArray(result) { return result.results || []; }
function touchService(env, batch, workspaceId, service, attrs, now) {
  batch.push(env.DB.prepare(
    "INSERT INTO telemetry_services(workspace_id,name,environment,region,first_seen_at,last_seen_at,sources_json,attributes_json) VALUES(?,?,?,?,?,?,?,?) ON CONFLICT(workspace_id,name) DO UPDATE SET last_seen_at=excluded.last_seen_at,attributes_json=excluded.attributes_json"
  ).bind(workspaceId, service, attrs["deployment.environment.name"] || null, attrs["cloud.region"] || null,
    new Date(now).toISOString(), new Date(now).toISOString(), '["otlp"]', JSON.stringify(attrs)));
}
async function listServices(env, workspaceId) {
  const result = await env.DB.prepare(
    "SELECT name, environment, region, first_seen_at, last_seen_at, sources_json, attributes_json FROM telemetry_services WHERE workspace_id=? AND last_seen_at >= ? ORDER BY name LIMIT ?"
  ).bind(workspaceId, new Date(Date.now() - 7 * 86400000).toISOString(), MAX_QUERY_ROWS).all();
  return { data: toArray(result).map(row => ({
    name: row.name, environment: row.environment, region: row.region,
    first_seen_at: row.first_seen_at, last_seen_at: row.last_seen_at,
    sources: JSON.parse(row.sources_json || "[]"),
    resource_attributes: JSON.parse(row.attributes_json || "{}")
  })), source: "cloudflare-d1", partial: false };
}
async function queryTelemetry(request, env, workspaceId, kind, url) {
  const { since } = parseWindow(url);
  const limit = safeLimit(url);
  const service = url.searchParams.get("service") || "";
  if (service.length > 128) throw Object.assign(new Error("Invalid service filter"), { status: 400 });
  if (kind === "metrics") {
    const result = await env.DB.prepare(
      "SELECT metric_name, metric_type, value, timestamp_ms, labels_json, service_name FROM metric_samples WHERE workspace_id=? AND timestamp_ms>=? AND (?='' OR service_name=?) ORDER BY timestamp_ms DESC LIMIT ?"
    ).bind(workspaceId, since, service, service, limit).all();
    return { data: toArray(result).map(row => ({ name: row.metric_name, type: row.metric_type, value: row.value, timestamp: new Date(row.timestamp_ms).toISOString(), labels: JSON.parse(row.labels_json), service: row.service_name })), source: "cloudflare-d1", queried_at: new Date().toISOString() };
  }
  if (kind === "logs") {
    const result = await env.DB.prepare(
      "SELECT service_name, timestamp_ms, severity_text, body, trace_id, span_id, attributes_json FROM log_records WHERE workspace_id=? AND timestamp_ms>=? AND (?='' OR service_name=?) ORDER BY timestamp_ms DESC LIMIT ?"
    ).bind(workspaceId, since, service, service, limit).all();
    return { data: toArray(result).map(row => ({ service: row.service_name, timestamp: new Date(row.timestamp_ms).toISOString(), severity: row.severity_text, body: row.body, trace_id: row.trace_id, span_id: row.span_id, attributes: JSON.parse(row.attributes_json) })), source: "cloudflare-d1", queried_at: new Date().toISOString() };
  }
  const traceId = url.searchParams.get("trace_id") || "";
  const result = await env.DB.prepare(
    "SELECT trace_id, span_id, parent_span_id, service_name, operation_name, start_time_ms, end_time_ms, status_code, attributes_json FROM spans WHERE workspace_id=? AND start_time_ms>=? AND (?='' OR service_name=?) AND (?='' OR trace_id=?) ORDER BY start_time_ms DESC LIMIT ?"
  ).bind(workspaceId, since, service, service, traceId, traceId, limit).all();
  return { data: toArray(result).map(row => ({ trace_id: row.trace_id, span_id: row.span_id, parent_span_id: row.parent_span_id, service: row.service_name, name: row.operation_name, start_time: new Date(row.start_time_ms).toISOString(), end_time: new Date(row.end_time_ms).toISOString(), status: row.status_code, attributes: JSON.parse(row.attributes_json) })), source: "cloudflare-d1", queried_at: new Date().toISOString() };
}
async function ingestOtlp(request, env, signal) {
  const bodyText = await readBoundedBody(request, MAX_BODY_BYTES);
  let payload;
  try { payload = JSON.parse(bodyText); } catch { throw Object.assign(new Error("This endpoint currently accepts OTLP/HTTP JSON encoding"), { status: 415 }); }
  const claims = await verifyJwt(bearer(request), env, env.WORKLOAD_OIDC_AUDIENCE || env.OIDC_AUDIENCE);
  const grant = await env.DB.prepare(
    "SELECT workspace_id FROM workload_grants WHERE issuer=? AND subject=? AND signal=? AND active=1"
  ).bind(claims.iss, claims.sub, signal).first();
  if (!grant) throw Object.assign(new Error("Workload has no grant for this signal"), { status: 403 });
  const workspaceId = grant.workspace_id;
  const now = Date.now();
  const batch = [];
  if (signal === "metrics") {
    for (const resource of payload.resourceMetrics || []) {
      const attrs = Object.fromEntries((resource.resource?.attributes || []).map(a => [a.key, a.value?.stringValue ?? a.value?.intValue ?? a.value?.boolValue]));
      const service = String(attrs["service.name"] || "unknown").slice(0,128);
      for (const scope of resource.scopeMetrics || []) for (const metric of scope.metrics || []) {
        const name = String(metric.name || "unknown").slice(0,160);
        const points = metric.gauge?.dataPoints || metric.sum?.dataPoints || metric.histogram?.dataPoints || [];
        for (const point of points.slice(0,500)) {
          const value = Number(point.asDouble ?? point.asInt ?? point.count ?? 0);
          const ts = Number(point.timeUnixNano || 0) / 1e6 || now;
          if (!Number.isFinite(value)) continue;
          batch.push(env.DB.prepare("INSERT INTO metric_samples(workspace_id,service_name,metric_name,metric_type,value,timestamp_ms,labels_json) VALUES(?,?,?,?,?,?,?)").bind(workspaceId,service,name,metric.type || "gauge",value,ts,JSON.stringify(attrs)));
        }
      }
      touchService(env, batch, workspaceId, service, attrs, now);
    }
  } else if (signal === "logs") {
    for (const resource of payload.resourceLogs || []) {
      const attrs = Object.fromEntries((resource.resource?.attributes || []).map(a => [a.key, a.value?.stringValue ?? a.value?.intValue ?? a.value?.boolValue]));
      const service = String(attrs["service.name"] || "unknown").slice(0,128);
      touchService(env, batch, workspaceId, service, attrs, now);
      for (const scope of resource.scopeLogs || []) for (const record of scope.logRecords || []) {
        const timestamp = Number(record.timeUnixNano || record.observedTimeUnixNano || 0) / 1e6 || now;
        const body = String(record.body?.stringValue ?? JSON.stringify(record.body || {})).slice(0,8192);
        const recordAttrs = Object.fromEntries((record.attributes || []).slice(0,32).map(a => [a.key, a.value?.stringValue ?? a.value?.intValue ?? a.value?.boolValue]));
        batch.push(env.DB.prepare("INSERT INTO log_records(workspace_id,service_name,timestamp_ms,severity_text,body,trace_id,span_id,attributes_json) VALUES(?,?,?,?,?,?,?,?)").bind(workspaceId,service,timestamp,String(record.severityText || ""),body,record.traceId || null,record.spanId || null,JSON.stringify(recordAttrs)));
      }
    }
  } else {
    for (const resource of payload.resourceSpans || []) {
      const attrs = Object.fromEntries((resource.resource?.attributes || []).map(a => [a.key, a.value?.stringValue ?? a.value?.intValue ?? a.value?.boolValue]));
      const service = String(attrs["service.name"] || "unknown").slice(0,128);
      touchService(env, batch, workspaceId, service, attrs, now);
      for (const scope of resource.scopeSpans || []) for (const span of scope.spans || []) {
        const start = Number(span.startTimeUnixNano || 0) / 1e6 || now;
        const end = Number(span.endTimeUnixNano || 0) / 1e6 || start;
        const spanAttrs = Object.fromEntries((span.attributes || []).slice(0,32).map(a => [a.key, a.value?.stringValue ?? a.value?.intValue ?? a.value?.boolValue]));
        batch.push(env.DB.prepare("INSERT OR REPLACE INTO spans(workspace_id,trace_id,span_id,parent_span_id,service_name,operation_name,start_time_ms,end_time_ms,status_code,attributes_json) VALUES(?,?,?,?,?,?,?,?,?,?)").bind(workspaceId,span.traceId,span.spanId,span.parentSpanId || null,service,String(span.name || "unknown").slice(0,256),start,end,span.status?.code === 2 ? "error" : "ok",JSON.stringify(spanAttrs)));
      }
    }
  }
  if (batch.length > 1000) throw Object.assign(new Error("OTLP request exceeds the 1000-record ingestion limit"), { status: 413 });
  for (let i = 0; i < batch.length; i += 100) await env.DB.batch(batch.slice(i, i + 100));
  return json({ partialSuccess: {}, accepted: batch.length });
}
export default {
  async fetch(request, env) {
    const id = requestId();
    const url = new URL(request.url);
    try {
      if (request.method === "OPTIONS") return cors(new Response(null, { status: 204 }), env);
      if (url.pathname === "/health/live") return cors(json({ status: "ok", request_id: id }), env);
      if (url.pathname === "/health/ready") {
        await env.DB.prepare("SELECT 1").first();
        return cors(json({ status: "ready", storage: "cloudflare-d1", request_id: id }), env);
      }
      if (url.pathname === API_PREFIX + "/meta") return cors(json({ name: "ORBIT Cloudflare Runtime", version: "0.1.0", runtime: "cloudflare-workers", capabilities: ["oidc","workspace-isolation","otlp-http-json","metrics","logs","traces"], request_id: id }), env);
      const otlp = url.pathname.match(/^\/v1\/otlp\/(metrics|logs|traces)$/);
      if (request.method === "POST" && otlp) return cors(await ingestOtlp(request, env, otlp[1]), env);
      const match = url.pathname.match(/^\/api\/v1\/workspaces\/([a-f0-9-]{36})\/telemetry\/(services|metrics|logs|traces)$/i);
      if (request.method === "GET" && match) {
        const [, workspaceId, kind] = match;
        await requireMember(request, env, workspaceId);
        const result = kind === "services" ? await listServices(env, workspaceId) : await queryTelemetry(request, env, workspaceId, kind, url);
        return cors(json({ ...result, request_id: id }), env);
      }
      return cors(fail(404, "route_not_found", "The requested ORBIT Cloudflare route was not found.", id), env);
    } catch (error) {
      const status = error.status || 500;
      if (status === 500) console.error(JSON.stringify({ event: "orbit_runtime_error", request_id: id, message: String(error.message || "unknown") }));
      return cors(fail(status, status === 401 ? "unauthorized" : status === 403 ? "forbidden" : status === 400 ? "invalid_request" : status === 413 ? "payload_too_large" : "runtime_error", status >= 500 ? "The ORBIT Cloudflare runtime could not complete the request." : error.message, id), env);
    }
  }
};
