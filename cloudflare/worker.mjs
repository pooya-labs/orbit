const API_PREFIX = "/api/v1/";
const ALLOWED_METHODS = new Set(["GET", "POST", "PUT", "PATCH", "DELETE"]);
const ALLOWED_HEADERS = new Set(["authorization", "content-type", "x-request-id", "traceparent", "tracestate"]);
const HOP_BY_HOP_HEADERS = [
  "connection", "keep-alive", "proxy-authenticate", "proxy-authorization",
  "te", "trailer", "transfer-encoding", "upgrade", "host"
];

function json(payload, status = 200, headers = {}) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: { "content-type": "application/json; charset=utf-8", "cache-control": "no-store", ...headers }
  });
}

function requestId() {
  return crypto.randomUUID();
}

function corsHeaders(origin, env) {
  const headers = new Headers({
    "cache-control": "no-store",
    "vary": "Origin"
  });
  if (origin === env.ALLOWED_ORIGIN && origin) {
    headers.set("access-control-allow-origin", origin);
    headers.set("access-control-expose-headers", "X-Request-ID, Retry-After");
  }
  return headers;
}

function isAllowedOrigin(request, env) {
  const origin = request.headers.get("origin");
  return !origin || origin === env.ALLOWED_ORIGIN;
}

function protectedOrigin(env) {
  if (!env.ORBIT_API_ORIGIN || !env.ORBIT_GATEWAY_SECRET) {
    throw new Error("The ORBIT API origin and gateway secret are not configured.");
  }
  let url;
  try {
    url = new URL(env.ORBIT_API_ORIGIN);
  } catch {
    throw new Error("The ORBIT API origin is invalid.");
  }
  if (url.protocol !== "https:" || url.username || url.password ||
      url.pathname !== "/" || url.search || url.hash) {
    throw new Error("The ORBIT API origin must be an HTTPS origin without credentials or a path.");
  }
  if (env.ORBIT_GATEWAY_SECRET.length < 64) {
    throw new Error("The ORBIT gateway secret is not configured safely.");
  }
  return url.origin;
}

function isAllowedPath(pathname) {
  return pathname === "/health/live" ||
    pathname === "/health/ready" ||
    pathname.startsWith(API_PREFIX);
}

function errorResponse(status, code, message, id, origin, env) {
  return json({ error: { code, message, request_id: id } }, status, corsHeaders(origin, env));
}

function handlePreflight(request, env, id, origin) {
  if (request.method !== "OPTIONS") return null;
  if (origin !== env.ALLOWED_ORIGIN) {
    return errorResponse(403, "origin_denied", "The browser origin is not allowed.", id, "", env);
  }
  const requestedMethod = request.headers.get("access-control-request-method") || "";
  if (!ALLOWED_METHODS.has(requestedMethod)) {
    return errorResponse(405, "method_not_allowed", "This request method is not supported.", id, origin, env);
  }
  const requestedHeaders = (request.headers.get("access-control-request-headers") || "")
    .split(",").map(value => value.trim().toLowerCase()).filter(Boolean);
  if (requestedHeaders.some(value => !ALLOWED_HEADERS.has(value))) {
    return errorResponse(400, "headers_not_allowed", "The requested browser headers are not allowed.", id, origin, env);
  }
  const headers = corsHeaders(origin, env);
  headers.set("access-control-allow-methods", [...ALLOWED_METHODS].join(", "));
  headers.set("access-control-allow-headers", [...ALLOWED_HEADERS].join(", "));
  headers.set("access-control-max-age", "600");
  return new Response(null, { status: 204, headers });
}

function buildUpstreamHeaders(request, env, id) {
  const headers = new Headers();
  for (const name of ["accept", "origin", "authorization", "content-type", "content-encoding", "traceparent", "tracestate"]) {
    const value = request.headers.get(name);
    if (value) headers.set(name, value);
  }
  const clientIp = request.headers.get("cf-connecting-ip") || "";
  if (clientIp && clientIp.length <= 64 && !/[\r\n,]/.test(clientIp)) {
    headers.set("X-ORBIT-Client-IP", clientIp);
  }
  headers.set("X-ORBIT-Gateway-Secret", env.ORBIT_GATEWAY_SECRET);
  headers.set("X-Request-ID", id);
  return headers;
}

function responseWithCors(upstream, env, origin, id) {
  const headers = new Headers(upstream.headers);
  for (const name of HOP_BY_HOP_HEADERS) headers.delete(name);
  for (const name of ["set-cookie", "access-control-allow-origin", "access-control-allow-credentials"]) {
    headers.delete(name);
  }
  headers.set("cache-control", "no-store");
  headers.set("vary", "Origin");
  headers.set("X-Request-ID", upstream.headers.get("X-Request-ID") || id);
  if (origin === env.ALLOWED_ORIGIN) {
    headers.set("access-control-allow-origin", origin);
    headers.set("access-control-expose-headers", "X-Request-ID, Retry-After");
  }
  return new Response(upstream.body, {
    status: upstream.status,
    statusText: upstream.statusText,
    headers
  });
}

async function proxyToOrbit(request, env, url, id, origin) {
  let apiOrigin;
  try {
    apiOrigin = protectedOrigin(env);
  } catch {
    return errorResponse(503, "origin_not_configured", "The Cloudflare gateway is deployed, but the ORBIT API origin has not been configured.", id, origin, env);
  }

  const upstreamUrl = new URL(url.pathname + url.search, apiOrigin);
  if (upstreamUrl.origin !== apiOrigin) {
    return errorResponse(400, "invalid_upstream_path", "The requested API path is invalid.", id, origin, env);
  }

  const headers = buildUpstreamHeaders(request, env, id);
  const init = {
    method: request.method,
    headers,
    redirect: "manual",
    signal: AbortSignal.timeout(15000)
  };
  if (request.method !== "GET" && request.method !== "HEAD") {
    init.body = request.body;
    init.duplex = "half";
  }

  let upstream;
  try {
    upstream = await fetch(new Request(upstreamUrl, init));
  } catch {
    return errorResponse(502, "api_origin_unavailable", "The ORBIT API origin could not be reached.", id, origin, env);
  }
  if (upstream.status >= 300 && upstream.status < 400) {
    return errorResponse(502, "unexpected_origin_redirect", "The ORBIT API origin returned an unexpected redirect.", id, origin, env);
  }
  return responseWithCors(upstream, env, origin, id);
}

export default {
  async fetch(request, env) {
    const id = requestId();
    const url = new URL(request.url);
    const origin = request.headers.get("origin") || "";

    if (!isAllowedOrigin(request, env)) {
      return errorResponse(403, "origin_denied", "The browser origin is not allowed.", id, "", env);
    }

    const preflight = handlePreflight(request, env, id, origin);
    if (preflight) return preflight;

    if (url.pathname === "/health/live" && request.method === "GET") {
      return json({ status: "ok", runtime: "cloudflare-edge", request_id: id }, 200, corsHeaders(origin, env));
    }

    if (!isAllowedPath(url.pathname)) {
      return errorResponse(404, "route_not_found", "The requested ORBIT gateway route was not found.", id, origin, env);
    }

    if (!ALLOWED_METHODS.has(request.method)) {
      return errorResponse(405, "method_not_allowed", "This request method is not supported.", id, origin, env);
    }

    return proxyToOrbit(request, env, url, id, origin);
  }
};
