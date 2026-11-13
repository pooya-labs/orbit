import test from "node:test";
import worker from "./worker.mjs";
import assert from "node:assert/strict";
import { exportJWK, generateKeyPair, SignJWT } from "jose";
import {
  incidentTransitionAllowed,
  parseBearer,
  readBoundedResponseBytes,
  roleAtLeast,
  validAccessTokenClaims,
  validIssuer,
  validUUID,
  validWorkspaceSlug
} from "./worker.mjs";

test("workspace slugs follow the API contract", () => {
  for (const value of ["ab", "orbit", "pooya-labs-2"]) assert.equal(validWorkspaceSlug(value), true);
  for (const value of ["", "a", "-orbit", "Orbit", "orbit-", "two words", "x".repeat(49)]) assert.equal(validWorkspaceSlug(value), false);
});

test("OIDC issuer must be an HTTPS origin/path without credentials, query or fragment", () => {
  assert.equal(validIssuer("https://identity.example.com/realms/orbit"), true);
  for (const value of ["http://identity.example.com", "https://u:p@example.com", "https://example.com?x=1", "not-a-url"]) {
    assert.equal(validIssuer(value), false);
  }
});

test("workspace role hierarchy is monotonic", () => {
  assert.equal(roleAtLeast("owner", "administrator"), true);
  assert.equal(roleAtLeast("administrator", "operator"), true);
  assert.equal(roleAtLeast("operator", "viewer"), true);
  assert.equal(roleAtLeast("viewer", "operator"), false);
  assert.equal(roleAtLeast("unknown", "viewer"), false);
});

test("incident lifecycle matches the Go domain transition policy", () => {
  assert.equal(incidentTransitionAllowed("open", "investigating"), true);
  assert.equal(incidentTransitionAllowed("open", "resolved"), true);
  assert.equal(incidentTransitionAllowed("investigating", "monitoring"), true);
  assert.equal(incidentTransitionAllowed("monitoring", "investigating"), true);
  assert.equal(incidentTransitionAllowed("resolved", "investigating"), true);
  assert.equal(incidentTransitionAllowed("resolved", "open"), false);
  assert.equal(incidentTransitionAllowed("open", "open"), false);
});

test("only an explicit Bearer token is accepted", () => {
  const make = (authorization) => new Request("https://api.example/api/v1/workspaces", { headers: authorization ? { authorization } : {} });
  assert.equal(parseBearer(make("Bearer eyJhbGciOiJSUzI1NiJ9.token.sig")), "eyJhbGciOiJSUzI1NiJ9.token.sig");
  assert.equal(parseBearer(make("bearer token")), "");
  assert.equal(parseBearer(make("Basic abc")), "");
  assert.equal(parseBearer(make("Bearer " + "x".repeat(16 * 1024))), "");
});

test("liveness is public and returns the native runtime marker", async () => {
  const response = await worker.fetch(
    new Request("https://native.example/health/live", { headers: { origin: "https://pooya-labs.github.io" } }),
    { ALLOWED_ORIGIN: "https://pooya-labs.github.io" }
  );
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("access-control-allow-origin"), "https://pooya-labs.github.io");
  assert.equal((await response.json()).runtime, "cloudflare-native");
});

test("readiness fails closed without a D1 binding", async () => {
  const response = await worker.fetch(
    new Request("https://native.example/health/ready"),
    { ALLOWED_ORIGIN: "https://pooya-labs.github.io" }
  );
  assert.equal(response.status, 503);
});

test("browser origins are exact-match only", async () => {
  const response = await worker.fetch(
    new Request("https://native.example/health/live", { headers: { origin: "https://attacker.example" } }),
    { ALLOWED_ORIGIN: "https://pooya-labs.github.io" }
  );
  assert.equal(response.status, 403);
  assert.equal(response.headers.get("access-control-allow-origin"), null);
});

test("API routes require a Bearer access token before handling data", async () => {
  const response = await worker.fetch(
    new Request("https://native.example/api/v1/workspaces", { headers: { origin: "https://pooya-labs.github.io" } }),
    { ALLOWED_ORIGIN: "https://pooya-labs.github.io", DB: {} }
  );
  assert.equal(response.status, 401);
  assert.equal((await response.json()).error.code, "unauthorized");
});

test("CORS preflight only accepts explicitly allowed headers and methods", async () => {
  const response = await worker.fetch(
    new Request("https://native.example/api/v1/workspaces", {
      method: "OPTIONS",
      headers: {
        origin: "https://pooya-labs.github.io",
        "access-control-request-method": "POST",
        "access-control-request-headers": "authorization,content-type"
      }
    }),
    { ALLOWED_ORIGIN: "https://pooya-labs.github.io" }
  );
  assert.equal(response.status, 204);
  assert.match(response.headers.get("access-control-allow-headers") || "", /authorization/);
});

test("workspace IDs use the canonical UUID shape", () => {
  assert.equal(validUUID("123e4567-e89b-12d3-a456-426614174000"), true);
  for (const value of ["123e4567-e89b-12d3-a456-42661417400", "123e4567-e89b-12d3-a456-42661417400z", "xxxxxxxx-xxxx-xxxx-xxxx-xxxxxxxxxxxx"]) {
    assert.equal(validUUID(value), false);
  }
});

test("OIDC access token validation aligns with the Go verifier contract", () => {
  const now = Math.floor(Date.now() / 1000);
  const payload = { iss: "https://identity.example.com/realms/orbit", sub: "user-123", iat: now, exp: now + 300, roles: ["viewer", "operator"] };
  const header = { alg: "RS256", kid: "key-1", typ: "JWT" };
  assert.equal(validAccessTokenClaims(payload, header, payload.iss, "roles"), true);
  assert.equal(validAccessTokenClaims({ ...payload, iat: undefined }, header, payload.iss, "roles"), false);
  assert.equal(validAccessTokenClaims({ ...payload, exp: now }, header, payload.iss, "roles"), false);
  assert.equal(validAccessTokenClaims(payload, { ...header, jku: "https://attacker.example/jwks" }, payload.iss, "roles"), false);
  assert.equal(validAccessTokenClaims(payload, { ...header, typ: "JWT+evil" }, payload.iss, "roles"), false);
  assert.equal(validAccessTokenClaims(payload, { alg: "RS256", typ: "JWT" }, payload.iss, "roles"), false);
  assert.equal(validAccessTokenClaims({ ...payload, custom_roles: 42 }, header, payload.iss, "custom_roles"), false);
  assert.equal(validAccessTokenClaims({ ...payload, custom_roles: ["operator"] }, header, payload.iss, "custom_roles"), true);
});

async function withSignedAccessToken(run) {
  const originalFetch = globalThis.fetch;
  const issuer = `https://oidc-${crypto.randomUUID()}.example/realm`;
  const audience = "orbit-api";
  const kid = "test-signing-key";
  const { privateKey, publicKey } = await generateKeyPair("RS256");
  const jwk = await exportJWK(publicKey);
  Object.assign(jwk, { kid, alg: "RS256", use: "sig", key_ops: ["verify"] });
  globalThis.fetch = async (input) => {
    const target = new URL(input instanceof Request ? input.url : String(input));
    if (target.href === issuer + "/.well-known/openid-configuration") {
      return new Response(JSON.stringify({ issuer, jwks_uri: issuer + "/jwks" }), {
        status: 200, headers: { "content-type": "application/json" }
      });
    }
    if (target.href === issuer + "/jwks") {
      return new Response(JSON.stringify({ keys: [jwk] }), {
        status: 200, headers: { "content-type": "application/json" }
      });
    }
    throw new Error("Unexpected test network request: " + target.href);
  };
  try {
    const token = await new SignJWT({ roles: ["viewer", "operator"] })
      .setProtectedHeader({ alg: "RS256", kid, typ: "JWT" })
      .setIssuer(issuer)
      .setSubject("native-api-test-user")
      .setAudience(audience)
      .setIssuedAt()
      .setExpirationTime("5m")
      .sign(privateKey);
    const allowRateLimit = { limit: async () => ({ success: true }) };
    await run(token, {
      ALLOWED_ORIGIN: "https://pooya-labs.github.io",
      ORBIT_OIDC_ISSUER: issuer,
      ORBIT_OIDC_AUDIENCE: audience,
      ORBIT_READ_LIMITER: allowRateLimit,
      ORBIT_WRITE_LIMITER: allowRateLimit
    });
  } finally {
    globalThis.fetch = originalFetch;
  }
}

function authorizedRequest(path, token) {
  return new Request("https://native.example" + path, {
    headers: {
      origin: "https://pooya-labs.github.io",
      authorization: "Bearer " + token
    }
  });
}

test("metadata response matches the direct ApiMetadata contract", async () => {
  await withSignedAccessToken(async (token, env) => {
    const response = await worker.fetch(authorizedRequest("/api/v1/meta", token), { ...env, DB: {} });
    assert.equal(response.status, 200);
    assert.deepEqual(await response.json(), {
      name: "orbit-api",
      version: "cloudflare-native.0.2.0",
      environment: "cloudflare-free"
    });
  });
});

test("global service inventory is restricted to the caller's workspace memberships", async () => {
  await withSignedAccessToken(async (token, env) => {
    let statement = "";
    let bindings = [];
    const DB = {
      prepare(sql) {
        statement = sql;
        return { bind(...values) {
          bindings = values;
          return { all: async () => ({ results: [{
            id: "service-visible",
            name: "api-gateway",
            description: "Demo service",
            environment: "production",
            region: "eu-west-1",
            status: "unknown",
            latency_p95_ms: null,
            error_rate: null,
            last_seen_at: null,
            source: "demo_seed"
          }] }) };
        } };
      }
    };
    const response = await worker.fetch(authorizedRequest("/api/v1/services", token), { ...env, DB });
    assert.equal(response.status, 200);
    assert.match(statement, /EXISTS\s*\(/);
    assert.match(statement, /workspace_memberships/);
    assert.deepEqual(bindings, [env.ORBIT_OIDC_ISSUER, "native-api-test-user"]);
    assert.deepEqual((await response.json()).data.map((item) => item.id), ["service-visible"]);
  });
});

test("global incident inventory applies the requested status and membership filter", async () => {
  await withSignedAccessToken(async (token, env) => {
    let statement = "";
    let bindings = [];
    const DB = {
      prepare(sql) {
        statement = sql;
        return { bind(...values) {
          bindings = values;
          return { all: async () => ({ results: [] }) };
        } };
      }
    };
    const response = await worker.fetch(authorizedRequest("/api/v1/incidents?status=monitoring", token), { ...env, DB });
    assert.equal(response.status, 200);
    assert.match(statement, /workspace_memberships/);
    assert.match(statement, /i\.status = \?/);
    assert.deepEqual(bindings, [env.ORBIT_OIDC_ISSUER, "native-api-test-user", "monitoring"]);
  });
});

test("creating a workspace provisions explicitly labeled demo services, incidents and alert rules atomically", async () => {
  await withSignedAccessToken(async (token, env) => {
    let batchStatements = [];
    const DB = {
      prepare(sql) {
        return {
          sql,
          bind(...values) { return { sql, values }; }
        };
      },
      async batch(statements) {
        batchStatements = statements;
        return statements.map(() => ({ meta: { changes: 1 } }));
      }
    };
    const request = new Request("https://native.example/api/v1/workspaces", {
      method: "POST",
      headers: {
        origin: "https://pooya-labs.github.io",
        authorization: "Bearer " + token,
        "content-type": "application/json"
      },
      body: JSON.stringify({ name: "ORBIT demo", slug: " ORBIT-DEMO " })
    });
    const response = await worker.fetch(request, { ...env, DB });
    assert.equal(response.status, 201);
    const payload = await response.json();
    assert.equal(payload.data.demo_seeded, true);
    assert.equal(payload.data.slug, "orbit-demo");
    assert.equal(batchStatements.length, 16);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO services")).length, 5);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO incidents")).length, 2);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO incident_timeline_events")).length, 2);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO alert_rules")).length, 2);
    assert.equal(batchStatements.filter((item) => item.sql.includes("\'demo_seed\'")).length, 7);
  });
});

test("OIDC metadata and JWKS bodies are bounded during streaming reads", async () => {
  const exact = new Response(new Uint8Array([1, 2, 3]), {
    headers: { "content-length": "3" }
  });
  assert.deepEqual([...await readBoundedResponseBytes(exact, 3)], [1, 2, 3]);

  const oversizedHeader = new Response(null, {
    headers: { "content-length": "1048577" }
  });
  await assert.rejects(readBoundedResponseBytes(oversizedHeader), /size limit/);

  const oversizedStream = new Response(new ReadableStream({
    start(controller) {
      controller.enqueue(new Uint8Array(700 * 1024));
      controller.enqueue(new Uint8Array(400 * 1024));
      controller.close();
    }
  }));
  await assert.rejects(readBoundedResponseBytes(oversizedStream), /size limit/);
});

test("workspace viewer membership can read but cannot perform operator mutations", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174000";
    let queryCount = 0;
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind(...values) {
            return {
              first: async () => {
                queryCount += 1;
                assert.deepEqual(values, [workspaceId, env.ORBIT_OIDC_ISSUER, "native-api-test-user"]);
                return {
                  workspace_id: workspaceId,
                  role: "viewer",
                  created_at: "2026-11-06T12:00:00.000Z",
                  updated_at: "2026-11-06T12:00:00.000Z"
                };
              }
            };
          }
        };
      }
    };
    const readResponse = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/membership`, token), { ...env, DB });
    assert.equal(readResponse.status, 200);
    assert.equal((await readResponse.json()).data.role, "viewer");

    const writeResponse = await worker.fetch(new Request(`https://native.example/api/v1/workspaces/${workspaceId}/incidents`, {
      method: "POST",
      headers: {
        origin: "https://pooya-labs.github.io",
        authorization: "Bearer " + token,
        "content-type": "application/json"
      },
      body: JSON.stringify({ title: "Forbidden write", description: "", service_id: "123e4567-e89b-12d3-a456-426614174001", severity: "low" })
    }), { ...env, DB });
    assert.equal(writeResponse.status, 403);
    assert.equal((await writeResponse.json()).error.code, "workspace_role_forbidden");
    assert.equal(queryCount, 2);
  });
});

test("workspace telemetry discovery and metric catalogue describe only the native runtime source", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174000";
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return {
              first: async () => ({
                workspace_id: workspaceId,
                role: "viewer",
                created_at: "2026-11-06T12:00:00.000Z",
                updated_at: "2026-11-06T12:00:00.000Z"
              })
            };
          }
        };
      }
    };

    const services = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/telemetry/services?limit=50`, token), { ...env, DB });
    assert.equal(services.status, 200);
    const service = (await services.json()).data[0];
    assert.equal(service.name, "orbit-native-runtime");
    assert.deepEqual(service.sources, ["workers-analytics-engine", "workers-observability"]);
    assert.equal(service.resource_attributes["service.name"][0], "orbit-native-runtime");

    const catalog = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/telemetry/metrics/catalog`, token), { ...env, DB });
    assert.equal(catalog.status, 200);
    assert.deepEqual((await catalog.json()).data.map((item) => item.id), [
      "request_rate", "successful_request_rate", "failed_request_rate"
    ]);
  });
});

test("workspace request metrics query Analytics Engine with a hashed tenant partition and return rates", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174000";
    const queries = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind(...values) {
            assert.equal(values[0], workspaceId);
            assert.equal(typeof values[1], "string");
            assert.equal(values[2], "native-api-test-user");
            return {
              first: async () => ({
                workspace_id: workspaceId,
                role: "viewer",
                created_at: "2026-11-06T12:00:00.000Z",
                updated_at: "2026-11-06T12:00:00.000Z"
              })
            };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query(request) {
        queries.push(request);
        return {
          data: [{
            bucket: new Date(Math.floor(Date.now() / 60_000) * 60_000).toISOString(),
            instance: "cloudflare-worker",
            service_version: "0.2.0",
            deployment_environment: "cloudflare",
            request_count: 120,
            successful_request_count: 114,
            failed_request_count: 6
          }],
          rows: 1,
          statistics: { elapsed_ms: 3, rows_read: 1, bytes_read: 128 }
        };
      }
    };

    const response = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/metrics?window=1h&service=orbit-native-runtime&group_by=instance,service_version,deployment_environment&metric=request_rate`,
      token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(response.status, 200);
    const payload = await response.json();
    assert.equal(payload.source, "workers-analytics-engine");
    assert.equal(payload.window, "1h");
    assert.equal(payload.step, "1m");
    assert.equal(payload.metric, "request_rate");
    assert.equal(payload.sampling, "adaptive");
    assert.equal(payload.data.length, 1);
    assert.deepEqual(payload.data[0].metric, {
      service_name: "orbit-native-runtime",
      instance: "cloudflare-worker",
      service_version: "0.2.0",
      deployment_environment: "cloudflare"
    });
    assert.equal(Number(payload.data[0].values[0][1]), 2);

    assert.equal(queries.length, 1);
    assert.match(queries[0].query, /events\.analyticsEngine\.orbit_native_requests/);
    assert.match(queries[0].query, /index1 = \$workspace_key/);
    assert.match(queries[0].query, /timestamp >= \$start/);
    assert.match(queries[0].query, /INTERVAL '[0-9]+' MINUTE/);
    assert.match(queries[0].query, /COUNT\(\*\) AS request_count/i);
    assert.match(queries[0].query, /countIf\(double1 >= 200 AND double1 < 400\) AS successful_request_count/i);
    assert.match(queries[0].query, /countIf\(double1 >= 400\) AS failed_request_count/i);
    assert.doesNotMatch(queries[0].query, /_sample_interval/i);
    assert.doesNotMatch(queries[0].query, new RegExp(workspaceId));
    assert.match(queries[0].params.workspace_key, /^[0-9a-f]{64}$/);
    assert.notEqual(queries[0].params.workspace_key, workspaceId);
    assert.equal(typeof queries[0].params.start, "string");
  });
});

test("workspace metrics can select successful and failed request rates from the same event schema", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174001";
    const DB = {
      prepare() {
        return {
          bind() {
            return {
              first: async () => ({ workspace_id: workspaceId, role: "viewer" })
            };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query() {
        return {
          data: [{
            bucket: "2026-11-08T00:00:00.000Z",
            request_count: 120,
            successful_request_count: 114,
            failed_request_count: 6
          }],
          rows: 1,
          statistics: { elapsed_ms: 1, rows_read: 1, bytes_read: 64 }
        };
      }
    };
    const successful = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/metrics?metric=successful_request_rate&group_by=none`, token
    ), { ...env, DB, ANALYTICS_SQL });
    const failed = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/metrics?metric=failed_request_rate&group_by=none`, token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(Number((await successful.json()).data[0].values[0][1]), 1.9);
    assert.equal(Number((await failed.json()).data[0].values[0][1]), 0.1);
  });
});

test("workspace telemetry metrics fail closed when Analytics SQL is not configured or fails", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174002";
    const DB = {
      prepare() {
        return {
          bind() {
            return { first: async () => ({ workspace_id: workspaceId, role: "viewer" }) };
          }
        };
      }
    };
    const missing = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/metrics`, token
    ), { ...env, DB });
    assert.equal(missing.status, 503);
    assert.equal((await missing.json()).error.code, "analytics_sql_not_configured");

    const unavailable = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/metrics`, token
    ), { ...env, DB, ANALYTICS_SQL: { async query() { throw new Error("private account failure"); } } });
    assert.equal(unavailable.status, 503);
    const payload = await unavailable.json();
    assert.equal(payload.error.code, "telemetry_query_unavailable");
    assert.doesNotMatch(payload.error.message, /private account failure/);
  });
});

test("rejected browser origins cannot write into a workspace telemetry partition", async () => {
  const workspaceId = "123e4567-e89b-12d3-a456-426614174099";
  const points = [];
  const spanAttributes = [];
  const capturedLogs = [];
  const originalLog = console.log;
  console.log = (event) => capturedLogs.push(event);
  let response;
  try {
    response = await worker.fetch(new Request(
      `https://native.example/api/v1/workspaces/${workspaceId}/membership`,
      { headers: { origin: "https://attacker.example" } }
    ), {
      ALLOWED_ORIGIN: "https://pooya-labs.github.io",
      ORBIT_REQUEST_EVENTS: { writeDataPoint(point) { points.push(point); } }
    }, {
      tracing: {
        getActiveSpan() {
          return { setAttribute(key, value) { spanAttributes.push([key, value]); } };
        }
      }
    });
  } finally {
    console.log = originalLog;
  }

  assert.equal(response.status, 403);
  assert.equal(spanAttributes.length, 0, "A rejected origin must never annotate a workspace trace");
  assert.equal(capturedLogs.length, 0, "A rejected origin must never emit a workspace-scoped log");
  assert.equal(points.length, 1);
  const globalDigest = new Uint8Array(await crypto.subtle.digest(
    "SHA-256",
    new TextEncoder().encode("orbit-native-telemetry:global")
  ));
  const globalKey = Array.from(globalDigest, (part) => part.toString(16).padStart(2, "0")).join("");
  assert.equal(points[0].indexes[0], globalKey);
  assert.notEqual(points[0].indexes[0], workspaceId);
  assert.equal(points[0].blobs.includes(workspaceId), false);
});

test("authenticated workspace validation failures remain in that workspace's request metrics", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174098";
    const points = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind(...values) {
            assert.deepEqual(values, [workspaceId, env.ORBIT_OIDC_ISSUER, "native-api-test-user"]);
            return {
              first: async () => ({
                workspace_id: workspaceId,
                role: "owner",
                created_at: "2026-11-06T12:00:00.000Z",
                updated_at: "2026-11-06T12:00:00.000Z"
              })
            };
          }
        };
      }
    };
    const request = new Request(
      `https://native.example/api/v1/workspaces/${workspaceId}/incidents`,
      {
        method: "POST",
        headers: {
          origin: "https://pooya-labs.github.io",
          authorization: "Bearer " + token,
          "content-type": "application/json"
        },
        body: JSON.stringify({})
      }
    );
    const response = await worker.fetch(request, {
      ...env,
      DB,
      ORBIT_REQUEST_EVENTS: { writeDataPoint(point) { points.push(point); } }
    });
    assert.equal(response.status, 400);
    assert.equal(points.length, 1);
    const digest = new Uint8Array(await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode(`orbit-native-telemetry:${workspaceId}`)
    ));
    const workspaceKey = Array.from(digest, (part) => part.toString(16).padStart(2, "0")).join("");
    assert.equal(points[0].indexes[0], workspaceKey);
    assert.equal(points[0].doubles[0], 400);
    assert.equal(points[0].blobs.includes(workspaceId), false);
  });
});

test("request telemetry writes one pseudonymous event without leaking path IDs, query strings or tokens", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174003";
    const points = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return {
              first: async () => ({ workspace_id: workspaceId, role: "viewer" })
            };
          }
        };
      }
    };
    const response = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/membership?private=query-value`, token
    ), {
      ...env,
      DB,
      ORBIT_REQUEST_EVENTS: { writeDataPoint(point) { points.push(point); } }
    });
    assert.equal(response.status, 200);
    assert.equal(points.length, 1);
    assert.match(points[0].indexes[0], /^[0-9a-f]{64}$/);
    assert.notEqual(points[0].indexes[0], workspaceId);
    assert.equal(points[0].blobs[0], "orbit-native-runtime");
    assert.equal(points[0].blobs[1], "/api/v1/workspaces/:workspace/membership");
    assert.equal(points[0].blobs[2], "GET");
    assert.equal(points[0].blobs.includes(workspaceId), false);
    assert.equal(points[0].blobs.includes("query-value"), false);
    assert.equal(points[0].blobs.includes(token), false);
    assert.equal(points[0].doubles[0], 200);
    assert.ok(points[0].doubles[1] >= 0);
  });
});

test("Analytics Engine write failures never change the API response", async () => {
  await withSignedAccessToken(async (token, env) => {
    const response = await worker.fetch(authorizedRequest("/health/live", token), {
      ...env,
      ORBIT_REQUEST_EVENTS: { writeDataPoint() { throw new Error("analytics unavailable"); } }
    });
    assert.equal(response.status, 200);
    assert.equal((await response.json()).runtime, "cloudflare-native");
  });
});

test("verified workspace requests emit safe structured Logs and tag only the active root span", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174077";
    const captured = [];
    const attributes = [];
    const originalLog = console.log;
    console.log = (event) => captured.push(event);
    try {
      const DB = {
        prepare(sql) {
          assert.match(sql, /FROM workspace_memberships/);
          return {
            bind() {
              return {
                first: async () => ({ workspace_id: workspaceId, role: "viewer" })
              };
            }
          };
        }
      };
      const span = {
        setAttribute(key, value) { attributes.push([key, value]); },
        spanContext() { return { traceId: "a".repeat(32), spanId: "b".repeat(16) }; }
      };
      const response = await worker.fetch(authorizedRequest(
        `/api/v1/workspaces/${workspaceId}/membership?private=do-not-log`, token
      ), { ...env, DB }, {
        tracing: { getActiveSpan: () => span }
      });
      assert.equal(response.status, 200);
    } finally {
      console.log = originalLog;
    }

    assert.equal(attributes.length, 1);
    assert.equal(attributes[0][0], "workspace_key");
    const digest = new Uint8Array(await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode(`orbit-native-telemetry:${workspaceId}`)
    ));
    const workspaceKey = Array.from(digest, (part) => part.toString(16).padStart(2, "0")).join("");
    assert.equal(attributes[0][1], workspaceKey);
    assert.equal(captured.length, 1);
    assert.equal(captured[0].event, "orbit.request");
    assert.equal(captured[0].workspace_key, workspaceKey);
    assert.equal(captured[0].route, "/api/v1/workspaces/:workspace/membership");
    assert.equal(captured[0].method, "GET");
    assert.equal(captured[0].status, 200);
    assert.equal(captured[0].trace_id, "a".repeat(32));
    assert.equal(captured[0].span_id, "b".repeat(16));
    const serialized = JSON.stringify(captured[0]);
    assert.doesNotMatch(serialized, new RegExp(workspaceId));
    assert.doesNotMatch(serialized, /do-not-log|authorization|Bearer|private=/i);
    assert.doesNotMatch(serialized, new RegExp(token));
  });
});

test("workspace Logs query only returns structured orbit.request events in the hashed tenant partition", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174076";
    const queries = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return { first: async () => ({ workspace_id: workspaceId, role: "viewer" }) };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query(request) {
        queries.push(request);
        return {
          data: [{
            timestamp: "2026-11-13T00:00:00.000Z",
            level: "info",
            message: "ORBIT request",
            traceId: "c".repeat(32),
            spanId: "d".repeat(16),
            requestId: "req-safe-1",
            route: "/api/v1/workspaces/:workspace/membership",
            method: "GET",
            status: 200,
            duration_ms: 12
          }],
          rows: 1,
          statistics: { elapsed_ms: 2, rows_read: 1, bytes_read: 256 }
        };
      }
    };
    const response = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/logs?window=1h&service=orbit-native-runtime&limit=25`, token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(response.status, 200);
    const payload = await response.json();
    assert.equal(payload.source, "workers-observability");
    assert.equal(payload.sampling, "adaptive");
    assert.equal(payload.limit, 25);
    assert.equal(payload.data.length, 1);
    assert.equal(payload.data[0].stream.level, "info");
    const line = JSON.parse(payload.data[0].values[0][1]);
    assert.equal(line.event, "orbit.request");
    assert.equal(line.trace_id, "c".repeat(32));
    assert.equal(line.status, 200);
    assert.equal(queries.length, 1);
    assert.match(queries[0].query, /FROM logs\.workersLogs/);
    assert.match(queries[0].query, /scriptName = 'orbit-native-runtime'/);
    assert.match(queries[0].query, /attributes\['event'\] = 'orbit\.request'/);
    assert.match(queries[0].query, /attributes\['workspace_key'\] = \$workspace_key/);
    assert.match(queries[0].query, /timestamp >= \$start/);
    assert.doesNotMatch(queries[0].query, /accountTag|authorization|request\.url|_sample_interval/i);
    const digest = new Uint8Array(await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode(`orbit-native-telemetry:${workspaceId}`)
    ));
    const workspaceKey = Array.from(digest, (part) => part.toString(16).padStart(2, "0")).join("");
    assert.equal(queries[0].params.workspace_key, workspaceKey);
    assert.equal(queries[0].params.workspace_key.includes(workspaceId), false);
  });
});

test("workspace Traces list only returns root spans tagged for the verified workspace", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174075";
    const queries = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return { first: async () => ({ workspace_id: workspaceId, role: "viewer" }) };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query(request) {
        queries.push(request);
        return {
          data: [{
            traceId: "e".repeat(32),
            serviceName: "orbit-native-runtime",
            startTime: "2026-11-13T00:00:00.000Z",
            durationMs: 18,
            spanStatus: "ok"
          }],
          rows: 1,
          statistics: { elapsed_ms: 2, rows_read: 1, bytes_read: 128 }
        };
      }
    };
    const response = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/traces?window=1h&service=orbit-native-runtime&limit=25`, token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(response.status, 200);
    const payload = await response.json();
    assert.equal(payload.source, "workers-observability");
    assert.equal(payload.sampling, "adaptive");
    assert.equal(payload.data[0].traceID, "e".repeat(32));
    assert.equal(payload.data[0].rootServiceName, "orbit-native-runtime");
    assert.equal(Number(payload.data[0].durationMs), 18);
    assert.match(queries[0].query, /FROM logs\.traces/);
    assert.match(queries[0].query, /serviceName = 'orbit-native-runtime'/);
    assert.match(queries[0].query, /attributes\['workspace_key'\] = \$workspace_key/);
    assert.match(queries[0].query, /parentSpanId IS NULL OR parentSpanId = ''/);
    assert.doesNotMatch(queries[0].query, /accountTag|_sample_interval/i);
    const digest = new Uint8Array(await crypto.subtle.digest(
      "SHA-256",
      new TextEncoder().encode(`orbit-native-telemetry:${workspaceId}`)
    ));
    const workspaceKey = Array.from(digest, (part) => part.toString(16).padStart(2, "0")).join("");
    assert.equal(queries[0].params.workspace_key, workspaceKey);
  });
});

test("workspace Trace detail verifies the root span ownership before reading child spans", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174074";
    const traceId = "f".repeat(32);
    const queries = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return { first: async () => ({ workspace_id: workspaceId, role: "viewer" }) };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query(request) {
        queries.push(request);
        if (queries.length === 1) return { data: [{ traceId }], rows: 1, statistics: {} };
        return {
          data: [
            {
              traceId, spanId: "1".repeat(16), parentSpanId: "",
              serviceName: "orbit-native-runtime",
              startTime: "2026-11-13T00:00:00.000Z",
              endTime: "2026-11-13T00:00:00.020Z",
              durationMs: 20, spanStatus: "ok"
            },
            {
              traceId, spanId: "2".repeat(16), parentSpanId: "1".repeat(16),
              serviceName: "orbit-native-runtime",
              startTime: "2026-11-13T00:00:00.005Z",
              endTime: "2026-11-13T00:00:00.010Z",
              durationMs: 5, spanStatus: "error"
            }
          ],
          rows: 2,
          statistics: { elapsed_ms: 2, rows_read: 2, bytes_read: 256 }
        };
      }
    };
    const response = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/traces/${traceId}`, token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(response.status, 200);
    const payload = await response.json();
    assert.equal(payload.source, "workers-observability");
    assert.equal(payload.trace_id, traceId);
    const spans = payload.data.batches[0].scopeSpans[0].spans;
    assert.equal(spans.length, 2);
    assert.equal(spans[0].name, "ORBIT request");
    assert.equal(spans[1].name, "Worker operation");
    assert.equal(spans[1].status.code, 2);
    assert.match(queries[0].query, /serviceName = 'orbit-native-runtime'/);
    assert.match(queries[0].query, /attributes\['workspace_key'\] = \$workspace_key/);
    assert.match(queries[1].query, /WHERE traceId = \$trace_id/);
    assert.match(queries[1].query, /serviceName = 'orbit-native-runtime'/);
    assert.doesNotMatch(queries[1].query, /attributes\['workspace_key'\]/);
    assert.equal(queries[0].params.trace_id, traceId);
    assert.equal(queries[1].params.trace_id, traceId);
    assert.notEqual(queries[0].params.workspace_key, workspaceId);
  });
});

test("a trace from another workspace is not readable even when its trace ID is known", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174073";
    const queries = [];
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return { first: async () => ({ workspace_id: workspaceId, role: "viewer" }) };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query(request) {
        queries.push(request);
        return { data: [], rows: 0, statistics: {} };
      }
    };
    const traceId = "9".repeat(32);
    const response = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/traces/${traceId}`, token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(response.status, 404);
    assert.equal((await response.json()).error.code, "telemetry_trace_not_found");
    assert.equal(queries.length, 1, "Child spans must never be queried before tenant ownership is proven");
  });
});

test("workspace Logs and Traces fail closed when the Analytics SQL binding is missing", async () => {
  await withSignedAccessToken(async (token, env) => {
    const globalTelemetry = await worker.fetch(authorizedRequest("/api/v1/telemetry/metrics?window=1h", token), { ...env, DB: {} });
    assert.equal(globalTelemetry.status, 503);
    assert.equal((await globalTelemetry.json()).error.code, "telemetry_source_unavailable");

    const workspaceId = "123e4567-e89b-12d3-a456-426614174004";
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return {
              first: async () => ({
                workspace_id: workspaceId,
                role: "owner",
                created_at: "2026-11-06T12:00:00.000Z",
                updated_at: "2026-11-06T12:00:00.000Z"
              })
            };
          }
        };
      }
    };
    const logs = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/telemetry/logs`, token), { ...env, DB });
    assert.equal(logs.status, 503);
    assert.equal((await logs.json()).error.code, "analytics_sql_not_configured");
    const traces = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/telemetry/traces`, token), { ...env, DB });
    assert.equal(traces.status, 503);
    assert.equal((await traces.json()).error.code, "analytics_sql_not_configured");

    const alerts = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/alerts?limit=50`, token), { ...env, DB });
    assert.equal(alerts.status, 503);
    assert.equal((await alerts.json()).error.code, "alert_evaluator_unavailable");

    const budget = await worker.fetch(authorizedRequest(`/api/v1/workspaces/${workspaceId}/slos/123e4567-e89b-12d3-a456-426614174001/budget`, token), { ...env, DB });
    assert.equal(budget.status, 503);
    assert.equal((await budget.json()).error.code, "slo_telemetry_unavailable");
  });
});


test("authenticated reads use the read limiter and stop before D1 when the budget is exceeded", async () => {
  await withSignedAccessToken(async (token, env) => {
    const keys = [];
    let databaseTouched = false;
    const response = await worker.fetch(authorizedRequest("/api/v1/workspaces", token), {
      ...env,
      ORBIT_READ_LIMITER: {
        async limit({ key }) {
          keys.push(key);
          return { success: false };
        }
      },
      ORBIT_WRITE_LIMITER: { async limit() { throw new Error("write limiter must not be used"); } },
      DB: {
        prepare() {
          databaseTouched = true;
          throw new Error("D1 must not be touched after a rejected rate-limit decision");
        }
      }
    });

    assert.equal(response.status, 429);
    assert.equal(response.headers.get("retry-after"), "60");
    assert.match(response.headers.get("access-control-expose-headers") || "", /Retry-After/i);
    assert.equal((await response.json()).error.code, "rate_limit_exceeded");
    assert.equal(keys.length, 1);
    assert.match(keys[0], /^[0-9a-f]{64}$/);
    assert.equal(keys[0].includes("native-api-test-user"), false);
    assert.equal(databaseTouched, false);
  });
});

test("authenticated writes use the write limiter and stop before D1 when the budget is exceeded", async () => {
  await withSignedAccessToken(async (token, env) => {
    let readChecks = 0;
    let databaseTouched = false;
    const request = new Request("https://native.example/api/v1/workspaces", {
      method: "POST",
      headers: {
        origin: "https://pooya-labs.github.io",
        authorization: "Bearer " + token,
        "content-type": "application/json"
      },
      body: JSON.stringify({ name: "Rate limited", slug: "rate-limited" })
    });
    const response = await worker.fetch(request, {
      ...env,
      ORBIT_READ_LIMITER: { async limit() { readChecks += 1; return { success: true }; } },
      ORBIT_WRITE_LIMITER: { async limit({ key }) { assert.match(key, /^[0-9a-f]{64}$/); return { success: false }; } },
      DB: {
        prepare() {
          databaseTouched = true;
          throw new Error("D1 must not be touched after a rejected rate-limit decision");
        }
      }
    });

    assert.equal(response.status, 429);
    assert.equal((await response.json()).error.code, "rate_limit_exceeded");
    assert.equal(readChecks, 0);
    assert.equal(databaseTouched, false);
  });
});

test("protected API requests fail closed when a rate-limit binding is absent", async () => {
  await withSignedAccessToken(async (token, env) => {
    let databaseTouched = false;
    const response = await worker.fetch(authorizedRequest("/api/v1/workspaces", token), {
      ...env,
      ORBIT_READ_LIMITER: undefined,
      DB: {
        prepare() {
          databaseTouched = true;
          throw new Error("D1 must not be touched without a rate-limit binding");
        }
      }
    });

    assert.equal(response.status, 503);
    assert.equal((await response.json()).error.code, "rate_limit_not_configured");
    assert.equal(databaseTouched, false);
  });
});

test("a rate-limit provider failure fails closed without reaching D1", async () => {
  await withSignedAccessToken(async (token, env) => {
    let databaseTouched = false;
    const response = await worker.fetch(authorizedRequest("/api/v1/workspaces", token), {
      ...env,
      ORBIT_READ_LIMITER: { async limit() { throw new Error("rate limit unavailable"); } },
      DB: {
        prepare() {
          databaseTouched = true;
          throw new Error("D1 must not be touched when the rate-limit provider fails");
        }
      }
    });

    assert.equal(response.status, 503);
    assert.equal((await response.json()).error.code, "rate_limit_unavailable");
    assert.equal(databaseTouched, false);
  });
});


test("workspace metrics fail closed instead of returning a truncated Analytics Engine result", async () => {
  await withSignedAccessToken(async (token, env) => {
    const workspaceId = "123e4567-e89b-12d3-a456-426614174099";
    const DB = {
      prepare(sql) {
        assert.match(sql, /FROM workspace_memberships/);
        return {
          bind() {
            return {
              first: async () => ({
                workspace_id: workspaceId,
                role: "viewer",
                created_at: "2026-11-06T12:00:00.000Z",
                updated_at: "2026-11-06T12:00:00.000Z"
              })
            };
          }
        };
      }
    };
    const ANALYTICS_SQL = {
      async query() {
        return { data: [], rows: 5000, statistics: { elapsed_ms: 3, rows_read: 5000, bytes_read: 1024 } };
      }
    };
    const result = await worker.fetch(authorizedRequest(
      `/api/v1/workspaces/${workspaceId}/telemetry/metrics?window=24h&group_by=instance,service_version,deployment_environment`,
      token
    ), { ...env, DB, ANALYTICS_SQL });
    assert.equal(result.status, 503);
    assert.equal((await result.json()).error.code, "telemetry_result_limit_exceeded");
  });
});
