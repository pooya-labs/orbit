import test from "node:test";
import worker from "./worker.mjs";
import assert from "node:assert/strict";
import { exportJWK, generateKeyPair, SignJWT } from "jose";
import {
  incidentTransitionAllowed,
  parseBearer,
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
    await run(token, { ALLOWED_ORIGIN: "https://pooya-labs.github.io", ORBIT_OIDC_ISSUER: issuer, ORBIT_OIDC_AUDIENCE: audience });
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
      body: JSON.stringify({ name: "ORBIT demo", slug: "orbit-demo" })
    });
    const response = await worker.fetch(request, { ...env, DB });
    assert.equal(response.status, 201);
    assert.equal((await response.json()).demo_seeded, true);
    assert.equal(batchStatements.length, 16);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO services")).length, 5);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO incidents")).length, 2);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO incident_timeline_events")).length, 2);
    assert.equal(batchStatements.filter((item) => item.sql.includes("INSERT INTO alert_rules")).length, 2);
    assert.equal(batchStatements.filter((item) => item.sql.includes("\'demo_seed\'")).length, 7);
  });
});
