import { createRemoteJWKSet, jwtVerify } from "jose";

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
  return typeof value === "string" && /^[a-z0-9](?:[a-z0-9-]{0,46}[a-z0-9])?$/.test(value);
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

async function getVerifier(env) {
  const issuer = String(env.ORBIT_OIDC_ISSUER || "").replace(/\/+$/, "");
  const audience = String(env.ORBIT_OIDC_AUDIENCE || "").trim();
  if (!issuer || !audience || !validIssuer(issuer)) {
    throw new Error("ORBIT OIDC issuer and audience are not configured safely.");
  }
  const cacheKey = issuer + "|" + audience;
  if (!VERIFIER_CACHE.has(cacheKey)) {
    const pending = (async () => {
      const discoveryUrl = issuer.replace(/\\/+$/, "") + "/.well-known/openid-configuration";
      const discoveryResponse = await fetch(discoveryUrl, { headers: { accept: "application/json" }, redirect: "error", signal: AbortSignal.timeout(5000) });
      if (!discoveryResponse.ok) throw new Error("OIDC discovery is unavailable.");
      const discovery = await discoveryResponse.json();
      if (!discovery || discovery.issuer !== issuer || typeof discovery.jwks_uri !== "string") {
        throw new Error("OIDC discovery issuer is invalid.");
      }
      const jwksUrl = new URL(discovery.jwks_uri);
      if (jwksUrl.protocol !== "https:" || jwksUrl.username || jwksUrl.password || jwksUrl.search || jwksUrl.hash) {
        throw new Error("OIDC JWKS endpoint must be HTTPS and contain no credentials.");
      }
      return {
        jwks: createRemoteJWKSet(jwksUrl, { timeoutDuration: 5000, cooldownDuration: 30_000, cacheMaxAge: 5 * 60_000 }),
        issuer,
        audience
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
    const { payload } = await jwtVerify(token, verifier.jwks, {
      issuer: verifier.issuer,
      audience: verifier.audience,
      algorithms: ["RS256", "ES256"],
      clockTolerance: 60
    });
    if (typeof payload.sub !== "string" || !payload.sub || payload.sub.length > 512 ||
        typeof payload.iss !== "string" || payload.iss !== verifier.issuer ||
        !Number.isFinite(payload.exp) || payload.exp <= (Date.now() / 1000) - 60) {
      return { error: "The access token is missing required claims." };
    }
    return { principal: { issuer: payload.iss, subject: payload.sub } };
  } catch {
    return { error: "The access token is invalid or expired." };
  }
}

async function readJson(request, maxBytes = MAX_BODY_BYTES) {
  const declaredLength = Number(request.headers.get("content-length") || 0);
  if (declaredLength > maxBytes) return { error: "Request body is too large." };
  const raw = await request.text();
  if (new TextEncoder().encode(raw).byteLength > maxBytes) return { error: "Request body is too large." };
  let value;
  try { value = JSON.parse(raw); } catch { return { error: "Request body must be valid JSON." }; }
  if (!value || Array.isArray(value) || typeof value !== "object") return { error: "Request body must be a JSON object." };
  return { value };
}

function auditStatement(db, workspaceId, actor, action, resourceType, resourceId, requestIdValue, details, now) {
  return db.prepare(`
    INSERT INTO workspace_audit_events
      (id, workspace_id, actor_issuer, actor_subject, action, resource_type, resource_id, request_id, details, created_at)
    VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
  `).bind(crypto.randomUUID(), workspaceId, actor.issuer, actor.subject, action, resourceType, resourceId || "", requestIdValue, JSON.stringify(details || {}), now);
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
    return response({ data: { name: "orbit-api", version: "cloudflare-native.0.1.0", environment: "cloudflare-free" } }, 200, origin, env, id);
  }

  if (path === "/api/v1/workspaces" && request.method === "GET") {
    const result = await env.DB.prepare(`
      SELECT w.id, w.name, w.slug, w.created_at, w.updated_at
      FROM workspaces w
      JOIN workspace_memberships m ON m.workspace_id = w.id
      WHERE m.user_issuer = ? AND m.user_subject = ?
      ORDER BY w.created_at ASC, w.id ASC
    `).bind(principal.issuer, principal.subject).all();
    return response({ data: result.results || [] }, 200, origin, env, id);
  }

  if (path === "/api/v1/workspaces" && request.method === "POST") {
    const parsed = await readJson(request, 8 * 1024);
    if (parsed.error) return fail(400, "invalid_workspace", parsed.error, origin, env, id);
    const { name, slug } = parsed.value;
    if (typeof name !== "string" || name.trim().length < 1 || name.trim().length > 120 || !validWorkspaceSlug(slug)) {
      return fail(400, "invalid_workspace", "Workspace name or slug is invalid.", origin, env, id);
    }
    const workspaceId = crypto.randomUUID();
    const auditId = crypto.randomUUID();
    try {
      await env.DB.batch([
        env.DB.prepare("INSERT INTO workspaces (id, name, slug, created_by_issuer, created_by_subject, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)")
          .bind(workspaceId, name.trim(), slug, principal.issuer, principal.subject, now, now),
        env.DB.prepare("INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role, created_at, updated_at) VALUES (?, ?, ?, 'owner', ?, ?)")
          .bind(workspaceId, principal.issuer, principal.subject, now, now),
        env.DB.prepare("INSERT INTO workspace_audit_actors (workspace_id, actor_issuer, actor_subject, actor_ref, created_at) VALUES (?, ?, ?, ?, ?)")
          .bind(workspaceId, principal.issuer, principal.subject, auditId, now),
        auditStatement(env.DB, workspaceId, principal, "workspace.created", "workspace", workspaceId, id, {}, now)
      ]);
    } catch {
      return fail(409, "workspace_slug_conflict", "That workspace slug is already in use or the workspace could not be created.", origin, env, id);
    }
    return response({ data: { id: workspaceId, name: name.trim(), slug, created_at: now, updated_at: now } }, 201, origin, env, id);
  }

  const match = path.match(/^\/api\/v1\/workspaces\/([0-9a-f-]{36})(?:\/(.*))?$/i);
  if (match) {
    const workspaceId = match[1];
    const suffix = match[2] || "";
    const requiredRole = request.method === "GET" ? "viewer" : "operator";
    const access = await requireWorkspace(env.DB, workspaceId, principal, requiredRole);
    if (access.error) return fail(access.error.status, access.error.code, access.error.message, origin, env, id);

    if (suffix === "membership" && request.method === "GET") {
      return response({ data: access.membership }, 200, origin, env, id);
    }

    if (suffix === "audit-events" && request.method === "GET") {
      const limit = limitFromUrl(url);
      if (!limit) return fail(400, "invalid_limit", "limit must be 10, 25, 50, or 100.", origin, env, id);
      const result = await env.DB.prepare(`
        SELECT id, workspace_id, actor_ref, actor_role, action, resource_type, resource_id, request_id, details, created_at
        FROM (
          SELECT e.id, e.workspace_id, a.actor_ref, m.role AS actor_role, e.action, e.resource_type, e.resource_id, e.request_id, e.details, e.created_at
          FROM workspace_audit_events e
          LEFT JOIN workspace_audit_actors a
            ON a.workspace_id = e.workspace_id AND a.actor_issuer = e.actor_issuer AND a.actor_subject = e.actor_subject
          LEFT JOIN workspace_memberships m
            ON m.workspace_id = e.workspace_id AND m.user_issuer = e.actor_issuer AND m.user_subject = e.actor_subject
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
          typeof serviceId !== "string" || !/^[0-9a-f-]{36}$/i.test(serviceId)) {
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

    const transition = suffix.match(/^incidents\/([0-9a-f-]{36})\/status$/i);
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
      const updated = await env.DB.prepare(`
        UPDATE incidents SET status = ?, updated_at = ?, resolved_at = ?
        WHERE id = ? AND workspace_id = ? AND status = ?
      `).bind(status, now, resolvedAt, incidentId, workspaceId, existing.status).run();
      if (!updated.meta?.changes) return fail(409, "incident_transition_conflict", "The incident changed before this update could be applied.", origin, env, id);
      const eventType = existing.status === "resolved" ? "incident.reopened" : "incident.status_changed";
      await env.DB.batch([
        env.DB.prepare("INSERT INTO incident_timeline_events (id, workspace_id, incident_id, actor_issuer, actor_subject, event_type, from_status, to_status, comment, request_id, details, created_at) VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, '{}', ?)")
          .bind(crypto.randomUUID(), workspaceId, incidentId, principal.issuer, principal.subject, eventType, existing.status, status, comment, id, now),
        auditStatement(env.DB, workspaceId, principal, eventType, "incident", incidentId, id, { from_status: existing.status, to_status: status, comment }, now)
      ]);
      return response({ data: { ...existing, status, updated_at: now, resolved_at: resolvedAt } }, 200, origin, env, id);
    }

    const timeline = suffix.match(/^incidents\/([0-9a-f-]{36})\/timeline$/i);
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
  roleAtLeast,
  validIssuer,
  validWorkspaceSlug
};
