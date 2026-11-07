import { createRemoteJWKSet, jwtVerify, customFetch } from "jose";

const API_PREFIX = "/api/v1/";
const METHODS = new Set(["GET", "HEAD", "POST", "PUT", "PATCH", "DELETE"]);
const ALLOWED_HEADERS = new Set(["authorization", "content-type", "x-request-id"]);
const MAX_BODY_BYTES = 16 * 1024;
const VERIFIER_CACHE = new Map();

function json(payload, status = 200, extraHeaders = {}) {
  return new Response(JSON.stringify(payload), {
    status,
    headers: {
      "content-type": "application/json; charset=utf-8",
      "cache-control": "no-store",
      ...extraHeaders
    }
  });
}

function requestId() {
  return crypto.randomUUID();
}

function cors(origin, env) {
  const headers = { "vary": "Origin", "cache-control": "no-store" };
  if (origin && origin === env.ALLOWED_ORIGIN) {
    headers["access-control-allow-origin"] = origin;
    headers["access-control-expose-headers"] = "X-Request-ID";
  }
  return headers;
}

function response(payload, status, origin, env, id) {
  return json(payload, status, { ...cors(origin, env), "x-request-id": id });
}

function fail(status, code, message, origin, env, id) {
  return response({ error: { code, message, request_id: id } }, status, origin, env, id);
}

function validIssuer(value) {
  try {
    const url = new URL(value);
    return url.protocol === "https:" && !url.username && !url.password && !url.search && !url.hash;
  } catch {
    return false;
  }
}

function validWorkspaceSlug(value) {
  return typeof value === "string" && /^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])$/.test(value);
}

function validUUID(value) {
  return typeof value === "string" && /^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(value);
}

function validAccessTokenClaims(payload, header, issuer, rolesClaim) {
  if (!payload || !header || typeof header !== "object") return false;
  if (["jku", "jwk", "x5u", "crit", "b64"].some((key) => Object.hasOwn(header, key))) return false;
  if (typeof header.kid !== "string" || !header.kid || header.kid.length > 256) return false;
  if (header.typ !== undefined && !["JWT", "at+JWT", "at+jwt"].includes(header.typ)) return false;
  const subject = payload.sub;
  const issuedAt = payload.iat;
  const expiresAt = payload.exp;
  if (typeof subject !== "string" || !subject.trim() || subject.length > 512) return false;
  if (payload.iss !== issuer || !Number.isFinite(issuedAt) || !Number.isFinite(expiresAt) ||
      expiresAt <= issuedAt || issuedAt > Date.now() / 1000 + 60) return false;
  const roles = payload[rolesClaim];
  if (roles !== undefined &&
      !(typeof roles === "string" || (Array.isArray(roles) && roles.every((role) => typeof role === "string")))) return false;
  return true;
}

function incidentTransitionAllowed(from, to) {
  const allowed = {
    open: ["investigating", "monitoring", "resolved"],
    investigating: ["monitoring", "resolved"],
    monitoring: ["investigating", "resolved"],
    resolved: ["investigating"]
  };
  return Boolean(allowed[from]?.includes(to));
}

function roleAtLeast(role, required) {
  const rank = { viewer: 1, operator: 2, administrator: 3, owner: 4 };
  return (rank[role] || 0) >= (rank[required] || 99);
}

function parseBearer(request) {
  const value = request.headers.get("authorization") || "";
  if (!value.startsWith("Bearer ") || value.length > 16 * 1024) return "";
  return value.slice(7).trim();
}

const MAX_OIDC_DOCUMENT_BYTES = 1024 * 1024;

async function readBoundedResponseBytes(response, maxBytes = MAX_OIDC_DOCUMENT_BYTES) {
  const declaredLength = response.headers.get("content-length");
  if (declaredLength !== null) {
    const parsedLength = Number(declaredLength);
    if (!Number.isSafeInteger(parsedLength) || parsedLength < 0 || parsedLength > maxBytes) {
      throw new Error("Remote OIDC document exceeds the configured size limit.");
    }
  }
  if (!response.body) return new Uint8Array();
  const reader = response.body.getReader();
  const chunks = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > maxBytes) {
        await reader.cancel();
        throw new Error("Remote OIDC document exceeds the configured size limit.");
      }
      chunks.push(value);
    }
  } catch (error) {
    try { await reader.cancel(); } catch {}
    throw error;
  }
  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }
  return bytes;
}

async function readOIDCJSON(url) {
  const response = await fetch(url, {
    headers: { accept: "application/json" },
    redirect: "error",
    signal: AbortSignal.timeout(5000)
  });
  if (!response.ok) throw new Error("OIDC metadata endpoint is unavailable.");
  const bytes = await readBoundedResponseBytes(response);
  const raw = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
  return JSON.parse(raw);
}

async function boundedJWKSFetch(url, options) {
  const response = await fetch(url, { ...options, redirect: "error" });
  if (response.status !== 200) return response;
  try {
    const bytes = await readBoundedResponseBytes(response);
    return new Response(bytes, { status: response.status, headers: response.headers });
  } catch {
    return new Response(null, { status: 413 });
  }
}

async function getVerifier(env) {
  const issuer = String(env.ORBIT_OIDC_ISSUER || "").trim();
  const audience = String(env.ORBIT_OIDC_AUDIENCE || "").trim();
  const rolesClaim = String(env.ORBIT_OIDC_ROLES_CLAIM || "roles").trim();
  if (!issuer || !audience || !validIssuer(issuer) || !/^[A-Za-z0-9_.-]{1,64}$/.test(rolesClaim)) {
    throw new Error("ORBIT OIDC issuer, audience and roles claim are not configured safely.");
  }
  const cacheKey = issuer + "|" + audience + "|" + rolesClaim;
  if (!VERIFIER_CACHE.has(cacheKey)) {
    const pending = (async () => {
      const discoveryUrl = issuer.replace(/\/+$/, "") + "/.well-known/openid-configuration";
      const discovery = await readOIDCJSON(discoveryUrl);
      if (!discovery || discovery.issuer !== issuer || typeof discovery.jwks_uri !== "string") {
        throw new Error("OIDC discovery issuer is invalid.");
      }
      const jwksUrl = new URL(discovery.jwks_uri);
      if (jwksUrl.protocol !== "https:" || jwksUrl.username || jwksUrl.password || jwksUrl.search || jwksUrl.hash) {
        throw new Error("OIDC JWKS endpoint must be HTTPS and contain no credentials, query or fragment.");
      }
      return {
        jwks: createRemoteJWKSet(jwksUrl, {
          timeoutDuration: 5000,
          cooldownDuration: 30_000,
          cacheMaxAge: 5 * 60_000,
          [customFetch]: boundedJWKSFetch
        }),
        issuer,
        audience,
        rolesClaim
      };
    })();
    VERIFIER_CACHE.set(cacheKey, pending);
    pending.catch(() => VERIFIER_CACHE.delete(cacheKey));
  }
  return VERIFIER_CACHE.get(cacheKey);
}

async function authenticate(request, env) {
  const token = parseBearer(request);
  if (!token) return { error: "A Bearer access token is required." };
  try {
    const verifier = await getVerifier(env);
    const { payload, protectedHeader } = await jwtVerify(token, verifier.jwks, {
      issuer: verifier.issuer,
      audience: verifier.audience,
      algorithms: ["RS256", "ES256"],
      clockTolerance: 60
    });
    if (!validAccessTokenClaims(payload, protectedHeader, verifier.issuer, verifier.rolesClaim)) {
      return { error: "The access token is missing required claims or has an unsupported JWT header." };
    }
    return { principal: { issuer: payload.iss, subject: payload.sub } };
  } catch {
    return { error: "The access token is invalid or expired." };
  }
}

async function rateLimitDecision(env, principal, method) {
  const binding = method === "GET" || method === "HEAD"
    ? env.ORBIT_READ_LIMITER
    : env.ORBIT_WRITE_LIMITER;
  if (!binding || typeof binding.limit !== "function") {
    return { configured: false, allowed: false };
  }

  // Do not send raw OIDC subjects or issuer values to the rate-limit service.
  const identity = new TextEncoder().encode(`${principal.issuer}\\0${principal.subject}`);
  const digest = new Uint8Array(await crypto.subtle.digest("SHA-256", identity));
  const key = Array.from(digest, (value) => value.toString(16).padStart(2, "0")).join("");
  const decision = await binding.limit({ key });
  return { configured: true, allowed: decision?.success === true };
}

async function readJson(request, maxBytes = MAX_BODY_BYTES) {
  const declaredLength = Number(request.headers.get("content-length") || 0);
  if (declaredLength > maxBytes) return { error: "Request body is too large." };
  if (!request.body) return { error: "Request body must be a JSON object." };

  const reader = request.body.getReader();
  const chunks = [];
  let total = 0;
  try {
    while (true) {
      const { done, value } = await reader.read();
      if (done) break;
      total += value.byteLength;
      if (total > maxBytes) {
        await reader.cancel();
        return { error: "Request body is too large." };
      }
      chunks.push(value);
    }
  } catch {
    return { error: "Request body could not be read." };
  }

  const bytes = new Uint8Array(total);
  let offset = 0;
  for (const chunk of chunks) {
    bytes.set(chunk, offset);
    offset += chunk.byteLength;
  }

  let value;
  try {
    const raw = new TextDecoder("utf-8", { fatal: true }).decode(bytes);
    value = JSON.parse(raw);
  } catch {
    return { error: "Request body must be valid UTF-8 JSON." };
  }
  if (!value || Array.isArray(value) || typeof value !== "object") return { error: "Request body must be a JSON object." };
  return { value };
}

function auditStatement(db, workspaceId, actor, action, resourceType, resourceId, requestIdValue, details, now) {
  return db.prepare(`
    INSERT INTO workspace_audit_events
      (id, workspace_id, actor_issuer, actor_subject, actor_role, action, resource_type, resource_id, request_id, details, created_at)
    SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?
    WHERE changes() = 1
  `).bind(crypto.randomUUID(), workspaceId, actor.issuer, actor.subject, actor.role || "owner", action, resourceType, resourceId || "", requestIdValue, JSON.stringify(details || {}), now);
}

async function membership(db, workspaceId, principal) {
  return db.prepare(`
    SELECT workspace_id, role, created_at, updated_at
    FROM workspace_memberships
    WHERE workspace_id = ? AND user_issuer = ? AND user_subject = ?
  `).bind(workspaceId, principal.issuer, principal.subject).first();
}

async function requireWorkspace(db, workspaceId, principal, role = "viewer") {
  const item = await membership(db, workspaceId, principal);
  if (!item) return { error: { status: 404, code: "workspace_not_found", message: "The workspace was not found." } };
  if (!roleAtLeast(item.role, role)) return { error: { status: 403, code: "workspace_role_forbidden", message: "Your workspace role cannot perform this operation." } };
  return { membership: item };
}

function limitFromUrl(url, defaultValue = 50) {
  const raw = url.searchParams.get("limit");
  if (!raw) return defaultValue;
  const value = Number(raw);
  return [10, 25, 50, 100].includes(value) ? value : 0;
}

async function handleApi(request, env, url, origin, id, principal) {
  if (!env.DB) return fail(503, "database_not_configured", "The Cloudflare D1 database is not bound to this Worker.", origin, env, id);
  const path = url.pathname;
  const now = new Date().toISOString();

  if (path === "/api/v1/meta" && request.method === "GET") {
    return response({ name: "orbit-api", version: "cloudflare-native.0.2.0", environment: "cloudflare-free" }, 200, origin, env, id);
  }

  if (path === "/api/v1/services" && request.method === "GET") {
    const result = await env.DB.prepare(`
      SELECT s.id, s.name, s.description, s.environment, s.region, s.status,
             s.latency_p95_ms, s.error_rate, s.last_seen_at, s.source
      FROM services s
      WHERE EXISTS (
        SELECT 1 FROM workspace_memberships m
        WHERE m.workspace_id = s.workspace_id
          AND m.user_issuer = ? AND m.user_subject = ?
      )
      ORDER BY s.name ASC, s.environment ASC LIMIT 500
    `).bind(principal.issuer, principal.subject).all();
    return response({ data: result.results || [] }, 200, origin, env, id);
  }

  if (path === "/api/v1/incidents" && request.method === "GET") {
    const status = url.searchParams.get("status") || "";
    if (status && !["open", "investigating", "monitoring", "resolved"].includes(status)) {
      return fail(400, "invalid_status", "status must be open, investigating, monitoring, or resolved.", origin, env, id);
    }
    const query = `
      SELECT i.id, i.title, i.description, i.service_id, s.name AS service_name,
             i.severity, i.status, i.started_at, i.updated_at, i.resolved_at, i.source
      FROM incidents i
      JOIN services s ON s.id = i.service_id AND s.workspace_id = i.workspace_id
      WHERE EXISTS (
        SELECT 1 FROM workspace_memberships m
        WHERE m.workspace_id = i.workspace_id
          AND m.user_issuer = ? AND m.user_subject = ?
      ) ${status ? "AND i.status = ?" : ""}
      ORDER BY i.started_at DESC, i.id DESC LIMIT 500
    `;
    const result = status
      ? await env.DB.prepare(query).bind(principal.issuer, principal.subject, status).all()
      : await env.DB.prepare(query).bind(principal.issuer, principal.subject).all();
    return response({ data: result.results || [] }, 200, origin, env, id);
  }

  if (path === "/api/v1/workspaces" && request.method === "GET") {
    const result = await env.DB.prepare(`
      SELECT w.id, w.name, w.slug, w.created_at, w.updated_at
      FROM workspaces w
      JOIN workspace_memberships m ON m.workspace_id = w.id
      WHERE m.user_issuer = ? AND m.user_subject = ?
      ORDER BY w.name ASC, w.id ASC LIMIT 100
    `).bind(principal.issuer, principal.subject).all();
    return response({ data: result.results || [] }, 200, origin, env, id);
  }

  if (path === "/api/v1/workspaces" && request.method === "POST") {
    const parsed = await readJson(request, 8 * 1024);
    if (parsed.error) return fail(400, "invalid_workspace", parsed.error, origin, env, id);
    const { name, slug } = parsed.value;
    const normalizedSlug = typeof slug === "string" ? slug.trim().toLowerCase() : "";
    if (typeof name !== "string" || name.trim().length < 1 || name.trim().length > 120 || !validWorkspaceSlug(normalizedSlug)) {
      return fail(400, "invalid_workspace", "Workspace name or slug is invalid.", origin, env, id);
    }
    const workspaceId = crypto.randomUUID();
    const auditId = crypto.randomUUID();
    const seededServices = [
      { name: "api-gateway", description: "Edge request router", environment: "production", region: "eu-central-1", status: "operational", latency: 42, errorRate: 0.0003 },
      { name: "identity-core", description: "Authentication service", environment: "production", region: "eu-central-1", status: "operational", latency: 68, errorRate: 0.0008 },
      { name: "billing-engine", description: "Payments and invoicing", environment: "production", region: "eu-west-1", status: "degraded", latency: 184, errorRate: 0.0142 },
      { name: "event-stream", description: "Asynchronous event processing", environment: "production", region: "eu-central-1", status: "operational", latency: 31, errorRate: 0.0001 },
      { name: "search-indexer", description: "Search indexing pipeline", environment: "production", region: "us-east-1", status: "incident", latency: null, errorRate: null }
    ].map((service) => ({ ...service, id: crypto.randomUUID() }));
    const serviceByName = Object.fromEntries(seededServices.map((service) => [service.name, service]));
    const seededIncidents = [
      { title: "Search indexing stalled", description: "The indexing queue stopped advancing after a shard rebalance. Synthetic demo data.", serviceName: "search-indexer", severity: "critical", status: "investigating", startedAt: new Date(Date.now() - 14 * 60 * 1000).toISOString(), updatedAt: new Date(Date.now() - 3 * 60 * 1000).toISOString(), comment: "Incident opened from the seeded demo scenario." },
      { title: "Elevated payment latency", description: "Payment API latency is above its normal baseline. Synthetic demo data.", serviceName: "billing-engine", severity: "medium", status: "monitoring", startedAt: new Date(Date.now() - 38 * 60 * 1000).toISOString(), updatedAt: new Date(Date.now() - 7 * 60 * 1000).toISOString(), comment: "Incident opened from the seeded demo scenario." }
    ].map((incident) => ({ ...incident, id: crypto.randomUUID(), service: serviceByName[incident.serviceName] }));
    const seededRules = [
      { name: "API error rate elevated", description: "Illustrative threshold for the demo API gateway. No live evaluator is configured in this runtime.", service_name: "api-gateway", metric: "request_rate", operator: "gt", threshold: 120, window_seconds: 300, pending_seconds: 120, severity: "high" },
      { name: "Billing traffic anomaly", description: "Illustrative rule for billing-engine traffic. No live evaluator is configured in this runtime.", service_name: "billing-engine", metric: "request_rate", operator: "gt", threshold: 80, window_seconds: 300, pending_seconds: 60, severity: "medium" }
    ].map((rule) => ({ ...rule, id: crypto.randomUUID() }));
    try {
      const statements = [
        env.DB.prepare("INSERT INTO workspaces (id, name, slug, created_by_issuer, created_by_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)")
          .bind(workspaceId, name.trim(), normalizedSlug, principal.issuer, principal.subject, now, now),
        env.DB.prepare("INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role, created_at, updated_at) VALUES (?, ?, ?, 'owner', ?, ?)")
          .bind(workspaceId, principal.issuer, principal.subject, now, now),
        env.DB.prepare("INSERT INTO workspace_audit_actors (workspace_id, actor_issuer, actor_subject, actor_ref, created_at) VALUES (?, ?, ?, ?, ?)")
          .bind(workspaceId, principal.issuer, principal.subject, auditId, now),
        auditStatement(env.DB, workspaceId, principal, "workspace.created", "workspace", workspaceId, id, {}, now)
      ];
      for (const service of seededServices) {
        statements.push(env.DB.prepare("INSERT INTO services (id, workspace_id, name, description, environment, region, status, latency_p95_ms, error_rate, last_seen_at, source, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 'demo_seed', ?, ?)")
          .bind(service.id, workspaceId, service.name, service.description, service.environment, service.region, service.status, service.latency, service.errorRate, now, now, now));
      }
      for (const incident of seededIncidents) {
        statements.push(env.DB.prepare("INSERT INTO incidents (id, workspace_id, title, description, service_id, severity, status, started_at, updated_at, resolved_at, source) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, NULL, 'demo_seed')")
          .bind(incident.id, workspaceId, incident.title, incident.description, incident.service.id, incident.severity, incident.status, incident.startedAt, incident.updatedAt));
        statements.push(env.DB.prepare("INSERT INTO incident_timeline_events (id, workspace_id, incident_id, actor_issuer, actor_subject, event_type, from_status, to_status, comment, request_id, details, created_at) VALUES (?, ?, ?, ?, ?, 'incident.created', NULL, ?, ?, 'demo-seed', ?, ?)")
          .bind(crypto.randomUUID(), workspaceId, incident.id, principal.issuer, principal.subject, incident.status, incident.comment, JSON.stringify({ source: "demo_seed" }), incident.startedAt));
      }
      for (const rule of seededRules) {
        statements.push(env.DB.prepare("INSERT INTO alert_rules (id, workspace_id, name, description, service_name, metric, operator, threshold, window_seconds, pending_seconds, severity, enabled, created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)")
          .bind(rule.id, workspaceId, rule.name, rule.description, rule.service_name, rule.metric, rule.operator, rule.threshold, rule.window_seconds, rule.pending_seconds, rule.severity, principal.issuer, principal.subject, principal.issuer, principal.subject, now, now));
      }
      statements.push(auditStatement(env.DB, workspaceId, principal, "workspace.demo_seeded", "workspace", workspaceId, id,
        { source: "demo_seed", services: seededServices.length, incidents: seededIncidents.length, alert_rules: seededRules.length }, now));
      await env.DB.batch(statements);
    } catch (error) {
      if (/UNIQUE constraint failed: workspaces[.]slug/i.test(String(error))) {
        return fail(409, "workspace_slug_conflict", "That workspace slug is already in use.", origin, env, id);
      }
      return fail(503, "workspace_create_failed", "The workspace could not be created.", origin, env, id);
    }
    return response({ data: { id: workspaceId, name: name.trim(), slug: normalizedSlug, created_at: now, updated_at: now, demo_seeded: true } }, 201, origin, env, id);
  }

  if (path.startsWith("/api/v1/telemetry/")) {
    return fail(503, "telemetry_source_unavailable", "This Cloudflare-native runtime does not have a Prometheus, Loki or Tempo source configured. The telemetry API remains in the Go/PostgreSQL profile.", origin, env, id);
  }

  const match = path.match(/^\/api\/v1\/workspaces\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:\/(.*))?$/i);
  if (match) {
    const workspaceId = match[1];
    const suffix = match[2] || "";
    const requiredRole = request.method === "GET" ? "viewer" : "operator";
    const access = await requireWorkspace(env.DB, workspaceId, principal, requiredRole);
    if (access.error) return fail(access.error.status, access.error.code, access.error.message, origin, env, id);
    principal.role = access.membership.role;

    if (suffix.startsWith("telemetry/")) {
      return fail(503, "telemetry_source_unavailable", "This Cloudflare-native runtime does not have a Prometheus, Loki or Tempo source configured. The telemetry API remains in the Go/PostgreSQL profile.", origin, env, id);
    }

    if (suffix === "membership" && request.method === "GET") {
      return response({ data: access.membership }, 200, origin, env, id);
    }

    if (suffix === "audit-events" && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const result = await env.DB.prepare(`
        SELECT id, workspace_id, actor_ref, actor_role, action, resource_type, resource_id, request_id, details, created_at
        FROM (
          SELECT e.id, e.workspace_id, a.actor_ref, e.actor_role, e.action, e.resource_type, e.resource_id, e.request_id, e.details, e.created_at
          FROM workspace_audit_events e
          LEFT JOIN workspace_audit_actors a
            ON a.workspace_id = e.workspace_id AND a.actor_issuer = e.actor_issuer AND a.actor_subject = e.actor_subject
          WHERE e.workspace_id = ?
          ORDER BY e.created_at DESC, e.id DESC
          LIMIT ?
        )
      `).bind(workspaceId, limit).all();
      const events = (result.results || []).map((event) => ({
        ...event,
        details: typeof event.details === "string" ? JSON.parse(event.details) : (event.details || {}),
        actor_ref: event.actor_ref || "unknown",
        actor_role: event.actor_role || "unknown"
      }));
      return response({ data: events }, 200, origin, env, id);
    }

    if (suffix === "services" && request.method === "GET") {
      const result = await env.DB.prepare(`
        SELECT id, name, description, environment, region, status, latency_p95_ms, error_rate, last_seen_at, source
        FROM services WHERE workspace_id = ? ORDER BY name ASC, environment ASC
      `).bind(workspaceId).all();
      return response({ data: result.results || [] }, 200, origin, env, id);
    }

    if (suffix === "incidents" && request.method === "GET") {
      const status = url.searchParams.get("status") || "";
      const allowed = new Set(["open", "investigating", "monitoring", "resolved"]);
      if (status && !allowed.has(status)) return fail(400, "invalid_incident_status", "The incident status filter is invalid.", origin, env, id);
      const query = `
        SELECT i.id, i.title, i.description, i.service_id, s.name AS service_name,
               i.severity, i.status, i.started_at, i.updated_at, i.resolved_at, i.source
        FROM incidents i
        JOIN services s ON s.id = i.service_id AND s.workspace_id = i.workspace_id
        WHERE i.workspace_id = ? ${status ? "AND i.status = ?" : ""}
        ORDER BY i.started_at DESC, i.id DESC LIMIT 100
      `;
      const result = status
        ? await env.DB.prepare(query).bind(workspaceId, status).all()
        : await env.DB.prepare(query).bind(workspaceId).all();
      return response({ data: result.results || [] }, 200, origin, env, id);
    }

    if (suffix === "incidents" && request.method === "POST") {
      const parsed = await readJson(request);
      if (parsed.error) return fail(400, "invalid_incident", parsed.error, origin, env, id);
      const input = parsed.value;
      const title = typeof input.title === "string" ? input.title.trim() : "";
      const description = typeof input.description === "string" ? input.description : "";
      const severity = input.severity;
      const serviceId = input.service_id;
      if (!title || title.length > 160 || description.length > 8000 ||
          !["critical", "high", "medium", "low"].includes(severity) ||
          typeof serviceId !== "string" || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(serviceId)) {
        return fail(400, "invalid_incident", "Incident fields are invalid.", origin, env, id);
      }
      const service = await env.DB.prepare("SELECT name FROM services WHERE id = ? AND workspace_id = ?").bind(serviceId, workspaceId).first();
      if (!service) return fail(404, "incident_resource_not_found", "The workspace or service was not found.", origin, env, id);
      const incidentId = crypto.randomUUID();
      const timelineId = crypto.randomUUID();
      try {
        await env.DB.batch([
          env.DB.prepare("INSERT INTO incidents (id, workspace_id, title, description, service_id, severity, status, started_at, updated_at, resolved_at, source) VALUES (?, ?, ?, ?, ?, ?, 'open', ?, ?, NULL, 'live')")
            .bind(incidentId, workspaceId, title, description, serviceId, severity, now, now),
          env.DB.prepare("INSERT INTO incident_timeline_events (id, workspace_id, incident_id, actor_issuer, actor_subject, event_type, from_status, to_status, comment, request_id, details, created_at) VALUES (?, ?, ?, ?, ?, 'incident.created', NULL, 'open', '', ?, '{}', ?)")
            .bind(timelineId, workspaceId, incidentId, principal.issuer, principal.subject, id, now),
          auditStatement(env.DB, workspaceId, principal, "incident.created", "incident", incidentId, id, { severity, service_id: serviceId }, now)
        ]);
      } catch {
        return fail(503, "incident_operation_unavailable", "The incident could not be created.", origin, env, id);
      }
      return response({ data: { id: incidentId, title, description, service_id: serviceId, service_name: service.name, severity, status: "open", started_at: now, updated_at: now, resolved_at: null, source: "live" } }, 201, origin, env, id);
    }

    const transition = suffix.match(/^incidents\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/status$/i);
    if (transition && request.method === "PATCH") {
      const parsed = await readJson(request);
      if (parsed.error) return fail(400, "invalid_incident_transition", parsed.error, origin, env, id);
      const status = parsed.value.status;
      const comment = typeof parsed.value.comment === "string" ? parsed.value.comment.trim() : "";
      if (!["open", "investigating", "monitoring", "resolved"].includes(status) || comment.length > 4000) {
        return fail(400, "invalid_incident_transition", "Status or comment is invalid.", origin, env, id);
      }
      const incidentId = transition[1];
      const existing = await env.DB.prepare(`
        SELECT i.id, i.title, i.description, i.service_id, s.name AS service_name,
               i.severity, i.status, i.started_at, i.updated_at, i.resolved_at, i.source
        FROM incidents i JOIN services s ON s.id = i.service_id AND s.workspace_id = i.workspace_id
        WHERE i.id = ? AND i.workspace_id = ?
      `).bind(incidentId, workspaceId).first();
      if (!existing) return fail(404, "incident_resource_not_found", "The workspace or incident was not found.", origin, env, id);
      if (!incidentTransitionAllowed(existing.status, status) || existing.status === status) {
        return fail(409, "incident_transition_conflict", "The requested incident status transition is not allowed.", origin, env, id);
      }
      const resolvedAt = status === "resolved" ? now : null;
      const eventType = existing.status === "resolved" ? "incident.reopened" : "incident.status_changed";
      const transitionResults = await env.DB.batch([
        env.DB.prepare(`
          UPDATE incidents SET status = ?, updated_at = ?, resolved_at = ?
          WHERE id = ? AND workspace_id = ? AND status = ?
        `).bind(status, now, resolvedAt, incidentId, workspaceId, existing.status),
        env.DB.prepare(`
          INSERT INTO incident_timeline_events
            (id, workspace_id, incident_id, actor_issuer, actor_subject, event_type, from_status, to_status, comment, request_id, details, created_at)
          SELECT ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?
          WHERE changes() = 1
        `).bind(crypto.randomUUID(), workspaceId, incidentId, principal.issuer, principal.subject, eventType, existing.status, status, comment, id, now),
        env.DB.prepare(`
          INSERT INTO workspace_audit_events
            (id, workspace_id, actor_issuer, actor_subject, actor_role, action, resource_type, resource_id, request_id, details, created_at)
          SELECT ?, ?, ?, ?, ?, ?, 'incident', ?, ?, ?, ?
          WHERE changes() = 1
        `).bind(crypto.randomUUID(), workspaceId, principal.issuer, principal.subject, principal.role, eventType, incidentId, id,
          JSON.stringify({ from_status: existing.status, to_status: status, comment }), now)
      ]);
      if (!transitionResults[0]?.meta?.changes) {
        return fail(409, "incident_transition_conflict", "The incident changed before this update could be applied.", origin, env, id);
      }
      return response({ data: { ...existing, status, updated_at: now, resolved_at: resolvedAt } }, 200, origin, env, id);
    }

    const timeline = suffix.match(/^incidents\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/timeline$/i);
    if (timeline && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const result = await env.DB.prepare(`
        SELECT id, workspace_id, incident_id, event_type, from_status, to_status, comment, request_id, details, created_at
        FROM incident_timeline_events
        WHERE workspace_id = ? AND incident_id = ?
        ORDER BY created_at DESC, id DESC LIMIT ?
      `).bind(workspaceId, timeline[1], limit).all();
      const events = result.results || [];
      return response({ data: events.map((event) => ({ ...event, details: typeof event.details === "string" ? JSON.parse(event.details) : (event.details || {}) })) }, 200, origin, env, id);
    }

    if (suffix === "alert-rules" && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const result = await env.DB.prepare("SELECT id, workspace_id, name, description, service_name, metric, operator, threshold, window_seconds, pending_seconds, severity, enabled, created_at, updated_at FROM alert_rules WHERE workspace_id = ? ORDER BY enabled DESC, updated_at DESC, id DESC LIMIT ?").bind(workspaceId, limit).all();
      return response({ data: (result.results || []).map((row) => ({ ...row, enabled: Boolean(row.enabled) })) }, 200, origin, env, id);
    }

    if (suffix === "alert-rules" && request.method === "POST") {
      const parsed = await readJson(request);
      if (parsed.error) return fail(400, "invalid_alert_rule", parsed.error, origin, env, id);
      const input = parsed.value;
      const name = typeof input.name === "string" ? input.name.trim() : "";
      const description = typeof input.description === "string" ? input.description.trim() : "";
      const serviceName = typeof input.service_name === "string" ? input.service_name.trim() : "";
      const { metric, operator, threshold, window_seconds: windowSeconds, pending_seconds: pendingSeconds, severity } = input;
      if (!name || name.length > 120 || description.length > 4000 ||
          (serviceName && !/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(serviceName)) ||
          metric !== "request_rate" || !["gt", "gte", "lt", "lte"].includes(operator) ||
          typeof threshold !== "number" || !Number.isFinite(threshold) || threshold <= 0 || threshold > 1000000000 ||
          ![60, 300, 900].includes(windowSeconds) ||
          !Number.isInteger(pendingSeconds) || pendingSeconds < 0 || pendingSeconds > 3600 ||
          !["critical", "high", "medium", "low"].includes(severity)) {
        return fail(400, "invalid_alert_rule", "Alert rule fields are invalid.", origin, env, id);
      }
      const ruleId = crypto.randomUUID();
      const details = { name, description, service_name: serviceName, metric, operator, threshold, window_seconds: windowSeconds, pending_seconds: pendingSeconds, severity, enabled: true };
      try {
        await env.DB.batch([
          env.DB.prepare("INSERT INTO alert_rules (id, workspace_id, name, description, service_name, metric, operator, threshold, window_seconds, pending_seconds, severity, enabled, created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)")
            .bind(ruleId, workspaceId, name, description, serviceName, metric, operator, threshold, windowSeconds, pendingSeconds, severity, principal.issuer, principal.subject, principal.issuer, principal.subject, now, now),
          auditStatement(env.DB, workspaceId, principal, "alert_rule.created", "alert_rule", ruleId, id, details, now)
        ]);
      } catch (error) {
        if (/unique constraint/i.test(String(error))) return fail(409, "alert_rule_name_conflict", "An alert rule with that name already exists in this workspace.", origin, env, id);
        throw error;
      }
      return response({ data: { id: ruleId, workspace_id: workspaceId, ...details, created_at: now, updated_at: now } }, 201, origin, env, id);
    }

    const ruleMatch = suffix.match(/^alert-rules\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:\/(enabled))?$/i);
    if (ruleMatch) {
      const ruleId = ruleMatch[1];
      if (ruleMatch[2] === "enabled" && request.method === "PATCH") {
        const parsed = await readJson(request);
        if (parsed.error || typeof parsed.value?.enabled !== "boolean") return fail(400, "invalid_alert_rule_state", "enabled must be a boolean.", origin, env, id);
        const current = await env.DB.prepare("SELECT id, enabled FROM alert_rules WHERE id = ? AND workspace_id = ?").bind(ruleId, workspaceId).first();
        if (!current) return fail(404, "alert_rule_not_found", "The workspace or alert rule was not found.", origin, env, id);
        if (Boolean(current.enabled) === parsed.value.enabled) return fail(409, "alert_rule_no_change", "The alert rule already has the requested state.", origin, env, id);
        await env.DB.batch([
          env.DB.prepare("UPDATE alert_rules SET enabled = ?, updated_by_issuer = ?, updated_by_subject = ?, updated_at = ? WHERE id = ? AND workspace_id = ?")
            .bind(parsed.value.enabled ? 1 : 0, principal.issuer, principal.subject, now, ruleId, workspaceId),
          auditStatement(env.DB, workspaceId, principal, parsed.value.enabled ? "alert_rule.enabled" : "alert_rule.disabled", "alert_rule", ruleId, id, { enabled: parsed.value.enabled }, now)
        ]);
        const updated = await env.DB.prepare("SELECT id, workspace_id, name, description, service_name, metric, operator, threshold, window_seconds, pending_seconds, severity, enabled, created_at, updated_at FROM alert_rules WHERE id = ? AND workspace_id = ?").bind(ruleId, workspaceId).first();
        return response({ data: { ...updated, enabled: Boolean(updated.enabled) } }, 200, origin, env, id);
      }
      if (request.method === "PUT" && !ruleMatch[2]) {
        const parsed = await readJson(request);
        if (parsed.error) return fail(400, "invalid_alert_rule", parsed.error, origin, env, id);
        const input = parsed.value;
        const name = typeof input.name === "string" ? input.name.trim() : "";
        const description = typeof input.description === "string" ? input.description.trim() : "";
        const serviceName = typeof input.service_name === "string" ? input.service_name.trim() : "";
        const { metric, operator, threshold, window_seconds: windowSeconds, pending_seconds: pendingSeconds, severity } = input;
        if (!name || name.length > 120 || description.length > 4000 ||
            (serviceName && !/^[A-Za-z0-9][A-Za-z0-9_.:-]{0,127}$/.test(serviceName)) ||
            metric !== "request_rate" || !["gt", "gte", "lt", "lte"].includes(operator) ||
            typeof threshold !== "number" || !Number.isFinite(threshold) || threshold <= 0 || threshold > 1000000000 ||
            ![60, 300, 900].includes(windowSeconds) ||
            !Number.isInteger(pendingSeconds) || pendingSeconds < 0 || pendingSeconds > 3600 ||
            !["critical", "high", "medium", "low"].includes(severity)) return fail(400, "invalid_alert_rule", "Alert rule fields are invalid.", origin, env, id);
        const current = await env.DB.prepare("SELECT id FROM alert_rules WHERE id = ? AND workspace_id = ?").bind(ruleId, workspaceId).first();
        if (!current) return fail(404, "alert_rule_not_found", "The workspace or alert rule was not found.", origin, env, id);
        const details = { name, description, service_name: serviceName, metric, operator, threshold, window_seconds: windowSeconds, pending_seconds: pendingSeconds, severity };
        try {
          await env.DB.batch([
            env.DB.prepare("UPDATE alert_rules SET name = ?, description = ?, service_name = ?, metric = ?, operator = ?, threshold = ?, window_seconds = ?, pending_seconds = ?, severity = ?, updated_by_issuer = ?, updated_by_subject = ?, updated_at = ? WHERE id = ? AND workspace_id = ?")
              .bind(name, description, serviceName, metric, operator, threshold, windowSeconds, pendingSeconds, severity, principal.issuer, principal.subject, now, ruleId, workspaceId),
            auditStatement(env.DB, workspaceId, principal, "alert_rule.updated", "alert_rule", ruleId, id, details, now)
          ]);
        } catch (error) {
          if (/unique constraint/i.test(String(error))) return fail(409, "alert_rule_name_conflict", "An alert rule with that name already exists in this workspace.", origin, env, id);
          throw error;
        }
        const updated = await env.DB.prepare("SELECT id, workspace_id, name, description, service_name, metric, operator, threshold, window_seconds, pending_seconds, severity, enabled, created_at, updated_at FROM alert_rules WHERE id = ? AND workspace_id = ?").bind(ruleId, workspaceId).first();
        return response({ data: { ...updated, enabled: Boolean(updated.enabled) } }, 200, origin, env, id);
      }
      if (request.method === "DELETE" && !ruleMatch[2]) {
        const current = await env.DB.prepare("SELECT id, name FROM alert_rules WHERE id = ? AND workspace_id = ?").bind(ruleId, workspaceId).first();
        if (!current) return fail(404, "alert_rule_not_found", "The workspace or alert rule was not found.", origin, env, id);
        const references = await env.DB.prepare("SELECT COUNT(*) AS count FROM alert_instances WHERE alert_rule_id = ? AND workspace_id = ?").bind(ruleId, workspaceId).first();
        if (Number(references?.count || 0) > 0) return fail(409, "alert_rule_in_use", "The alert rule has alert history and cannot be deleted.", origin, env, id);
        await env.DB.batch([
          env.DB.prepare("DELETE FROM alert_rules WHERE id = ? AND workspace_id = ?").bind(ruleId, workspaceId),
          auditStatement(env.DB, workspaceId, principal, "alert_rule.deleted", "alert_rule", ruleId, id, { name: current.name }, now)
        ]);
        return new Response(null, { status: 204, headers: { ...cors(origin, env), "x-request-id": id } });
      }
    }

    if (suffix === "alerts" && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const status = url.searchParams.get("status") || "";
      if (status && !["pending", "firing", "resolved", "cancelled"].includes(status)) return fail(400, "invalid_alert_status", "Alert status filter is invalid.", origin, env, id);
      return fail(503, "alert_evaluator_unavailable", "Alert rules are stored, but live evaluation requires a real telemetry source that is not configured for this Cloudflare-only runtime.", origin, env, id);
    }

    if (suffix === "deployments" && request.method === "GET") {
      const limit = limitFromUrl(url, 100);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const serviceId = url.searchParams.get("service_id") || "";
      if (serviceId && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(serviceId)) return fail(400, "invalid_deployment_filter", "Service identifier is invalid.", origin, env, id);
      const result = await env.DB.prepare("SELECT id, workspace_id, service_id, service_name, environment, version, commit_sha, change_reference, release_notes, deployed_at, created_at FROM deployment_annotations WHERE workspace_id = ? AND (? = '' OR service_id = ?) ORDER BY deployed_at DESC, id DESC LIMIT ?").bind(workspaceId, serviceId, serviceId, limit).all();
      return response({ data: result.results || [] }, 200, origin, env, id);
    }

    if (suffix === "deployments" && request.method === "POST") {
      const parsed = await readJson(request);
      if (parsed.error) return fail(400, "invalid_deployment_annotation", parsed.error, origin, env, id);
      const input = parsed.value;
      const serviceId = input.service_id;
      const version = typeof input.version === "string" ? input.version.trim() : "";
      const commitSha = typeof input.commit_sha === "string" ? input.commit_sha.trim().toLowerCase() : "";
      const changeReference = typeof input.change_reference === "string" ? input.change_reference.trim() : "";
      const releaseNotes = typeof input.release_notes === "string" ? input.release_notes.trim() : "";
      const deployedTimestamp = typeof input.deployed_at === "string" ? Date.parse(input.deployed_at) : (input.deployed_at === undefined ? Date.now() : NaN);
      if (typeof serviceId !== "string" || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(serviceId) ||
          !/^[A-Za-z0-9][A-Za-z0-9.+_:/-]{0,127}$/.test(version) ||
          (commitSha && !/^[0-9a-f]{7,64}$/.test(commitSha)) ||
          (changeReference && !/^[A-Za-z0-9][A-Za-z0-9._:/#-]{0,255}$/.test(changeReference)) ||
          releaseNotes.length > 4000 || !Number.isFinite(deployedTimestamp) || deployedTimestamp > Date.now() + 300000) return fail(400, "invalid_deployment_annotation", "Deployment annotation fields are invalid.", origin, env, id);
      const service = await env.DB.prepare("SELECT name, environment FROM services WHERE id = ? AND workspace_id = ?").bind(serviceId, workspaceId).first();
      if (!service) return fail(404, "deployment_annotation_service_not_found", "The workspace service was not found.", origin, env, id);
      const deploymentId = crypto.randomUUID();
      const at = new Date(deployedTimestamp).toISOString();
      const details = { service_id: serviceId, service_name: service.name, environment: service.environment, version, commit_sha: commitSha, change_reference: changeReference, deployed_at: at };
      await env.DB.batch([
        env.DB.prepare("INSERT INTO deployment_annotations (id, workspace_id, service_id, service_name, environment, version, commit_sha, change_reference, release_notes, deployed_at, created_by_issuer, created_by_subject, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)")
          .bind(deploymentId, workspaceId, serviceId, service.name, service.environment, version, commitSha, changeReference, releaseNotes, at, principal.issuer, principal.subject, now),
        auditStatement(env.DB, workspaceId, principal, "deployment.annotation_created", "deployment_annotation", deploymentId, id, details, now)
      ]);
      return response({ data: { id: deploymentId, workspace_id: workspaceId, service_id: serviceId, service_name: service.name, environment: service.environment, version, commit_sha: commitSha, change_reference: changeReference, release_notes: releaseNotes, deployed_at: at, created_at: now } }, 201, origin, env, id);
    }

    if (suffix === "deployments/compare" && request.method === "GET") {
      const fromId = url.searchParams.get("from_id") || "";
      const toId = url.searchParams.get("to_id") || "";
      if (!/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(fromId) || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(toId) || fromId === toId) return fail(400, "invalid_deployment_comparison", "Compare two different deployment annotations.", origin, env, id);
      const [from, to] = await Promise.all([
        env.DB.prepare("SELECT id, workspace_id, service_id, service_name, environment, version, commit_sha, change_reference, release_notes, deployed_at, created_at FROM deployment_annotations WHERE id = ? AND workspace_id = ?").bind(fromId, workspaceId).first(),
        env.DB.prepare("SELECT id, workspace_id, service_id, service_name, environment, version, commit_sha, change_reference, release_notes, deployed_at, created_at FROM deployment_annotations WHERE id = ? AND workspace_id = ?").bind(toId, workspaceId).first()
      ]);
      if (!from || !to || from.service_id !== to.service_id || Date.parse(from.deployed_at) >= Date.parse(to.deployed_at)) return fail(400, "invalid_deployment_comparison", "Compare two deployments of the same service in chronological order.", origin, env, id);
      return response({ from, to, elapsed_seconds: Math.floor((Date.parse(to.deployed_at) - Date.parse(from.deployed_at)) / 1000), version_changed: from.version !== to.version, commit_changed: from.commit_sha !== to.commit_sha }, 200, origin, env, id);
    }

    if (suffix === "slos" && request.method === "GET") {
      const limit = limitFromUrl(url, 100);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const serviceId = url.searchParams.get("service_id") || "";
      if (serviceId && !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(serviceId)) return fail(400, "invalid_slo_filter", "Service identifier is invalid.", origin, env, id);
      const result = await env.DB.prepare("SELECT slo.id, slo.workspace_id, slo.service_id, services.name AS service_name, services.environment, slo.name, slo.route, slo.target, slo.rolling_window_days, slo.enabled, slo.created_at, slo.updated_at FROM service_level_objectives slo JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id WHERE slo.workspace_id = ? AND (? = '' OR slo.service_id = ?) ORDER BY slo.enabled DESC, slo.updated_at DESC, slo.id DESC LIMIT ?").bind(workspaceId, serviceId, serviceId, limit).all();
      return response({ data: (result.results || []).map((row) => ({ ...row, enabled: Boolean(row.enabled) })) }, 200, origin, env, id);
    }

    if (suffix === "slos" && request.method === "POST") {
      const parsed = await readJson(request);
      if (parsed.error) return fail(400, "invalid_slo", parsed.error, origin, env, id);
      const input = parsed.value;
      const serviceId = input.service_id;
      const name = typeof input.name === "string" ? input.name.trim() : "";
      const route = typeof input.route === "string" ? input.route.trim() : "";
      const target = input.target;
      const windowDays = input.rolling_window_days;
      if (typeof serviceId !== "string" || !/^[0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12}$/i.test(serviceId) ||
          !name || name.length > 120 || !/^\/[A-Za-z0-9][A-Za-z0-9/_-]{0,127}$/.test(route) || route.includes("//") ||
          typeof target !== "number" || !Number.isFinite(target) || target < 0.90 || target > 0.9999 || ![1, 3, 7].includes(windowDays)) return fail(400, "invalid_slo", "SLO fields are invalid.", origin, env, id);
      const service = await env.DB.prepare("SELECT name, environment FROM services WHERE id = ? AND workspace_id = ?").bind(serviceId, workspaceId).first();
      if (!service) return fail(404, "slo_service_not_found", "The workspace service was not found.", origin, env, id);
      const sloId = crypto.randomUUID();
      const details = { service_id: serviceId, service_name: service.name, environment: service.environment, name, route, target, rolling_window_days: windowDays, enabled: true };
      try {
        await env.DB.batch([
          env.DB.prepare("INSERT INTO service_level_objectives (id, workspace_id, service_id, name, route, target, rolling_window_days, enabled, created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?, 1, ?, ?, ?, ?, ?, ?)")
            .bind(sloId, workspaceId, serviceId, name, route, target, windowDays, principal.issuer, principal.subject, principal.issuer, principal.subject, now, now),
          auditStatement(env.DB, workspaceId, principal, "slo.created", "slo_definition", sloId, id, details, now)
        ]);
      } catch (error) {
        if (/unique constraint/i.test(String(error))) return fail(409, "slo_name_conflict", "An SLO with that name already exists for this service.", origin, env, id);
        throw error;
      }
      return response({ data: { id: sloId, workspace_id: workspaceId, service_id: serviceId, service_name: service.name, environment: service.environment, name, route, target, rolling_window_days: windowDays, enabled: true, created_at: now, updated_at: now } }, 201, origin, env, id);
    }

    const sloMatch = suffix.match(/^slos\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})(?:\/(enabled|budget))?$/i);
    if (sloMatch) {
      const sloId = sloMatch[1];
      if (sloMatch[2] === "budget" && request.method === "GET") return fail(503, "slo_telemetry_unavailable", "SLO definitions are stored, but error budgets require real request telemetry that is not configured for this Cloudflare-only runtime.", origin, env, id);
      if (sloMatch[2] === "enabled" && request.method === "PATCH") {
        const parsed = await readJson(request);
        if (parsed.error || typeof parsed.value?.enabled !== "boolean") return fail(400, "invalid_slo_state", "enabled must be provided as a boolean.", origin, env, id);
        const current = await env.DB.prepare("SELECT id, enabled FROM service_level_objectives WHERE id = ? AND workspace_id = ?").bind(sloId, workspaceId).first();
        if (!current) return fail(404, "slo_not_found", "The workspace or SLO was not found.", origin, env, id);
        if (Boolean(current.enabled) === parsed.value.enabled) return fail(409, "slo_no_change", "The SLO already has the requested state.", origin, env, id);
        await env.DB.batch([
          env.DB.prepare("UPDATE service_level_objectives SET enabled = ?, updated_by_issuer = ?, updated_by_subject = ?, updated_at = ? WHERE id = ? AND workspace_id = ?").bind(parsed.value.enabled ? 1 : 0, principal.issuer, principal.subject, now, sloId, workspaceId),
          auditStatement(env.DB, workspaceId, principal, parsed.value.enabled ? "slo.enabled" : "slo.disabled", "slo_definition", sloId, id, { enabled: parsed.value.enabled }, now)
        ]);
        const updated = await env.DB.prepare("SELECT slo.id, slo.workspace_id, slo.service_id, services.name AS service_name, services.environment, slo.name, slo.route, slo.target, slo.rolling_window_days, slo.enabled, slo.created_at, slo.updated_at FROM service_level_objectives slo JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id WHERE slo.id = ? AND slo.workspace_id = ?").bind(sloId, workspaceId).first();
        return response({ data: { ...updated, enabled: Boolean(updated.enabled) } }, 200, origin, env, id);
      }
      if (request.method === "PUT" && !sloMatch[2]) {
        const parsed = await readJson(request);
        if (parsed.error) return fail(400, "invalid_slo", parsed.error, origin, env, id);
        const input = parsed.value;
        const name = typeof input.name === "string" ? input.name.trim() : "";
        const route = typeof input.route === "string" ? input.route.trim() : "";
        const target = input.target;
        const windowDays = input.rolling_window_days;
        if (!name || name.length > 120 || !/^\/[A-Za-z0-9][A-Za-z0-9/_-]{0,127}$/.test(route) || route.includes("//") ||
            typeof target !== "number" || !Number.isFinite(target) || target < 0.90 || target > 0.9999 || ![1, 3, 7].includes(windowDays)) return fail(400, "invalid_slo", "SLO fields are invalid.", origin, env, id);
        const current = await env.DB.prepare("SELECT id FROM service_level_objectives WHERE id = ? AND workspace_id = ?").bind(sloId, workspaceId).first();
        if (!current) return fail(404, "slo_not_found", "The workspace or SLO was not found.", origin, env, id);
        try {
          await env.DB.batch([
            env.DB.prepare("UPDATE service_level_objectives SET name = ?, route = ?, target = ?, rolling_window_days = ?, updated_by_issuer = ?, updated_by_subject = ?, updated_at = ? WHERE id = ? AND workspace_id = ?").bind(name, route, target, windowDays, principal.issuer, principal.subject, now, sloId, workspaceId),
            auditStatement(env.DB, workspaceId, principal, "slo.updated", "slo_definition", sloId, id, { name, route, target, rolling_window_days: windowDays }, now)
          ]);
        } catch (error) {
          if (/unique constraint/i.test(String(error))) return fail(409, "slo_name_conflict", "An SLO with that name already exists for this service.", origin, env, id);
          throw error;
        }
        const updated = await env.DB.prepare("SELECT slo.id, slo.workspace_id, slo.service_id, services.name AS service_name, services.environment, slo.name, slo.route, slo.target, slo.rolling_window_days, slo.enabled, slo.created_at, slo.updated_at FROM service_level_objectives slo JOIN services ON services.id = slo.service_id AND services.workspace_id = slo.workspace_id WHERE slo.id = ? AND slo.workspace_id = ?").bind(sloId, workspaceId).first();
        return response({ data: { ...updated, enabled: Boolean(updated.enabled) } }, 200, origin, env, id);
      }
      if (request.method === "DELETE" && !sloMatch[2]) {
        const current = await env.DB.prepare("SELECT id, name FROM service_level_objectives WHERE id = ? AND workspace_id = ?").bind(sloId, workspaceId).first();
        if (!current) return fail(404, "slo_not_found", "The workspace or SLO was not found.", origin, env, id);
        await env.DB.batch([
          env.DB.prepare("DELETE FROM service_level_objectives WHERE id = ? AND workspace_id = ?").bind(sloId, workspaceId),
          auditStatement(env.DB, workspaceId, principal, "slo.deleted", "slo_definition", sloId, id, { name: current.name }, now)
        ]);
        return new Response(null, { status: 204, headers: { ...cors(origin, env), "x-request-id": id } });
      }
    }

    if (suffix === "alert-notification-channels" && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const result = await env.DB.prepare("SELECT id, workspace_id, name, adapter, secret_ref, enabled, created_at, updated_at FROM alert_notification_channels WHERE workspace_id = ? ORDER BY enabled DESC, created_at DESC, id DESC LIMIT ?").bind(workspaceId, limit).all();
      const channels = (result.results || []).map((row) => ({ id: row.id, workspace_id: row.workspace_id, name: row.name, adapter: row.adapter, enabled: Boolean(row.enabled), secret_configured: typeof env[row.secret_ref] === "string" && env[row.secret_ref].length > 0, created_at: row.created_at, updated_at: row.updated_at }));
      return response({ data: channels }, 200, origin, env, id);
    }

    if (suffix === "alert-notification-channels" && request.method === "POST") {
      const parsed = await readJson(request);
      if (parsed.error) return fail(400, "invalid_alert_notification", parsed.error, origin, env, id);
      const name = typeof parsed.value.name === "string" ? parsed.value.name.trim() : "";
      const secretRef = typeof parsed.value.secret_ref === "string" ? parsed.value.secret_ref.trim() : "";
      if (!name || name.length > 80 || !/^[A-Z][A-Z0-9_]{0,63}$/.test(secretRef)) return fail(400, "invalid_alert_notification", "Channel name or secret reference is invalid.", origin, env, id);
      const channelId = crypto.randomUUID();
      try {
        await env.DB.batch([
          env.DB.prepare("INSERT INTO alert_notification_channels (id, workspace_id, name, adapter, secret_ref, enabled, created_by_issuer, created_by_subject, updated_by_issuer, updated_by_subject, created_at, updated_at) VALUES (?, ?, ?, 'webhook', ?, 1, ?, ?, ?, ?, ?, ?)")
            .bind(channelId, workspaceId, name, secretRef, principal.issuer, principal.subject, principal.issuer, principal.subject, now, now),
          auditStatement(env.DB, workspaceId, principal, "alert_notification_channel.created", "alert_notification_channel", channelId, id, { name, adapter: "webhook" }, now)
        ]);
      } catch (error) {
        if (/unique constraint/i.test(String(error))) return fail(409, "alert_notification_channel_conflict", "A notification channel with that name already exists.", origin, env, id);
        throw error;
      }
      return response({ data: { id: channelId, workspace_id: workspaceId, name, adapter: "webhook", enabled: true, secret_configured: typeof env[secretRef] === "string" && env[secretRef].length > 0, created_at: now, updated_at: now } }, 201, origin, env, id);
    }

    const channelState = suffix.match(/^alert-notification-channels\/([0-9a-f]{8}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{4}-[0-9a-f]{12})\/enabled$/i);
    if (channelState && request.method === "PATCH") {
      const parsed = await readJson(request);
      if (parsed.error || typeof parsed.value?.enabled !== "boolean") return fail(400, "invalid_alert_notification_state", "enabled must be provided as a boolean.", origin, env, id);
      const channelId = channelState[1];
      const current = await env.DB.prepare("SELECT id, name, enabled FROM alert_notification_channels WHERE id = ? AND workspace_id = ?").bind(channelId, workspaceId).first();
      if (!current) return fail(404, "alert_notification_resource_not_found", "The workspace or notification channel was not found.", origin, env, id);
      if (Boolean(current.enabled) === parsed.value.enabled) return fail(409, "alert_notification_no_change", "The notification channel already has the requested state.", origin, env, id);
      await env.DB.batch([
        env.DB.prepare("UPDATE alert_notification_channels SET enabled = ?, updated_by_issuer = ?, updated_by_subject = ?, updated_at = ? WHERE id = ? AND workspace_id = ?").bind(parsed.value.enabled ? 1 : 0, principal.issuer, principal.subject, now, channelId, workspaceId),
        auditStatement(env.DB, workspaceId, principal, parsed.value.enabled ? "alert_notification_channel.enabled" : "alert_notification_channel.disabled", "alert_notification_channel", channelId, id, { enabled: parsed.value.enabled, name: current.name }, now)
      ]);
      const updated = await env.DB.prepare("SELECT id, workspace_id, name, adapter, secret_ref, enabled, created_at, updated_at FROM alert_notification_channels WHERE id = ? AND workspace_id = ?").bind(channelId, workspaceId).first();
      return response({ data: { id: updated.id, workspace_id: updated.workspace_id, name: updated.name, adapter: updated.adapter, enabled: Boolean(updated.enabled), secret_configured: typeof env[updated.secret_ref] === "string" && env[updated.secret_ref].length > 0, created_at: updated.created_at, updated_at: updated.updated_at } }, 200, origin, env, id);
    }

    if (suffix === "alert-notification-deliveries" && request.method === "GET") return fail(503, "alert_delivery_unavailable", "Notification channel settings are stored, but deliveries require live alert evaluation and a configured delivery worker.", origin, env, id);
    if (/^alert-notification-deliveries\/[0-9a-f-]{36}\/attempts$/i.test(suffix) && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      return fail(503, "alert_delivery_unavailable", "Delivery attempts are unavailable until the notification delivery worker is configured.", origin, env, id);
    }
  }

  return fail(404, "route_not_found", "The requested ORBIT API route was not found in the Cloudflare-native runtime.", origin, env, id);
}

export default {
  async fetch(request, env) {
    const id = requestId();
    const url = new URL(request.url);
    const origin = request.headers.get("origin") || "";

    if (origin && origin !== env.ALLOWED_ORIGIN) return fail(403, "origin_denied", "The browser origin is not allowed.", "", env, id);
    if (request.method === "OPTIONS") {
      if (!origin || origin !== env.ALLOWED_ORIGIN) return fail(403, "origin_denied", "The browser origin is not allowed.", "", env, id);
      const requestedMethod = request.headers.get("access-control-request-method") || "";
      const requestedHeaders = (request.headers.get("access-control-request-headers") || "").split(",").map((value) => value.trim().toLowerCase()).filter(Boolean);
      if (!METHODS.has(requestedMethod) || requestedHeaders.some((header) => !ALLOWED_HEADERS.has(header))) {
        return fail(400, "preflight_denied", "The requested browser method or headers are not allowed.", origin, env, id);
      }
      return new Response(null, {
        status: 204,
        headers: {
          ...cors(origin, env),
          "access-control-allow-methods": [...METHODS].join(", "),
          "access-control-allow-headers": [...ALLOWED_HEADERS].join(", "),
          "access-control-max-age": "600"
        }
      });
    }

    if (url.pathname === "/health/live" && request.method === "GET") {
      return response({ status: "ok", runtime: "cloudflare-native", request_id: id }, 200, origin, env, id);
    }
    if (url.pathname === "/health/ready" && request.method === "GET") {
      try {
        if (!env.DB) throw new Error("D1 binding missing");
        await env.DB.prepare("SELECT 1 AS ready").first();
        return response({ status: "ready", database: "d1", request_id: id }, 200, origin, env, id);
      } catch {
        return fail(503, "database_not_ready", "The Cloudflare D1 database is not ready.", origin, env, id);
      }
    }
    if (!url.pathname.startsWith(API_PREFIX)) return fail(404, "route_not_found", "The requested route was not found.", origin, env, id);
    if (!METHODS.has(request.method)) return fail(405, "method_not_allowed", "The request method is not supported.", origin, env, id);
    if (!env.DB) return fail(503, "database_not_configured", "The Cloudflare D1 database is not bound to this Worker.", origin, env, id);

    const auth = await authenticate(request, env);
    if (!auth.principal) return fail(401, "unauthorized", auth.error || "Authentication is required.", origin, env, id);

    let rateLimit;
    try {
      rateLimit = await rateLimitDecision(env, auth.principal, request.method);
    } catch {
      return fail(503, "rate_limit_unavailable", "The Cloudflare rate-limit service is temporarily unavailable.", origin, env, id);
    }
    if (!rateLimit.configured) {
      return fail(503, "rate_limit_not_configured", "The Cloudflare rate-limit bindings are not configured.", origin, env, id);
    }
    if (!rateLimit.allowed) {
      return json({
        error: { code: "rate_limit_exceeded", message: "The request rate limit was exceeded.", request_id: id }
      }, 429, { ...cors(origin, env), "x-request-id": id, "retry-after": "60" });
    }

    try {
      return await handleApi(request, env, url, origin, id, auth.principal);
    } catch {
      return fail(503, "api_operation_unavailable", "The ORBIT API operation is temporarily unavailable.", origin, env, id);
    }
  }
};

export {
  incidentTransitionAllowed,
  parseBearer,
  readBoundedResponseBytes,
  roleAtLeast,
  validAccessTokenClaims,
  validIssuer,
  validUUID,
  validWorkspaceSlug
};
