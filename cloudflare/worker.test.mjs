import test from "node:test";
import assert from "node:assert/strict";
import gateway from "./worker.mjs";

const ORIGIN = "https://pooya-labs.github.io";
const API_ORIGIN = "https://orbit-api.example.test";
const SECRET = "a".repeat(64);

function env(overrides = {}) {
  return {
    ALLOWED_ORIGIN: ORIGIN,
    ORBIT_API_ORIGIN: API_ORIGIN,
    ORBIT_GATEWAY_SECRET: SECRET,
    ...overrides
  };
}

test("health liveness is local to the Worker and does not contact the origin", async () => {
  const response = await gateway.fetch(new Request("https://orbit-runtime.workers.dev/health/live"), env());
  assert.equal(response.status, 200);
  assert.deepEqual(await response.json().then(body => [body.status, body.runtime]), ["ok", "cloudflare-edge"]);
});

test("API routes fail closed until the Go API origin and gateway secret are configured", async () => {
  const response = await gateway.fetch(
    new Request("https://orbit-runtime.workers.dev/api/v1/meta", { headers: { origin: ORIGIN } }),
    env({ ORBIT_API_ORIGIN: "", ORBIT_GATEWAY_SECRET: "" })
  );
  assert.equal(response.status, 503);
  assert.equal((await response.json()).error.code, "origin_not_configured");
  assert.equal(response.headers.get("access-control-allow-origin"), ORIGIN);
});

test("unknown paths are not an open proxy", async () => {
  const response = await gateway.fetch(new Request("https://orbit-runtime.workers.dev/anything"), env());
  assert.equal(response.status, 404);
  assert.equal((await response.json()).error.code, "route_not_found");
});

test("preflight accepts only the Pages origin and documented headers", async () => {
  const allowed = await gateway.fetch(new Request("https://orbit-runtime.workers.dev/api/v1/meta", {
    method: "OPTIONS",
    headers: {
      origin: ORIGIN,
      "access-control-request-method": "GET",
      "access-control-request-headers": "authorization,content-type"
    }
  }), env());
  assert.equal(allowed.status, 204);
  assert.equal(allowed.headers.get("access-control-allow-origin"), ORIGIN);

  const denied = await gateway.fetch(new Request("https://orbit-runtime.workers.dev/api/v1/meta", {
    method: "OPTIONS",
    headers: {
      origin: "https://attacker.example",
      "access-control-request-method": "GET"
    }
  }), env());
  assert.equal(denied.status, 403);
});

test("API proxy preserves bearer auth and query but replaces spoofable gateway metadata", async () => {
  const originalFetch = globalThis.fetch;
  let captured;
  globalThis.fetch = async request => {
    captured = request;
    return new Response(JSON.stringify({ name: "orbit-api", version: "1.0.0", environment: "production" }), {
      status: 200,
      headers: {
        "content-type": "application/json",
        "set-cookie": "must-not-cross-edge=1",
        "access-control-allow-origin": "https://wrong.example"
      }
    });
  };

  try {
    const request = new Request("https://orbit-runtime.workers.dev/api/v1/meta?source=cloudflare", {
      headers: {
        origin: ORIGIN,
        authorization: "Bearer user-access-token",
        "x-orbit-gateway-secret": "attacker-value",
        "x-orbit-client-ip": "203.0.113.99"
      }
    });
    const wrapped = new Request(request);
    // Cloudflare supplies this header at the edge; a caller-supplied value is not forwarded.
    const headers = new Headers(wrapped.headers);
    headers.set("cf-connecting-ip", "198.51.100.24");
    const response = await gateway.fetch(new Request(wrapped, { headers }), env());

    assert.equal(response.status, 200);
    assert.equal(new URL(captured.url).origin, API_ORIGIN);
    assert.equal(new URL(captured.url).pathname, "/api/v1/meta");
    assert.equal(new URL(captured.url).search, "?source=cloudflare");
    assert.equal(captured.headers.get("authorization"), "Bearer user-access-token");
    assert.equal(captured.headers.get("origin"), ORIGIN);
    assert.equal(captured.headers.get("x-orbit-gateway-secret"), SECRET);
    assert.equal(captured.headers.get("x-orbit-client-ip"), "198.51.100.24");
    assert.match(captured.headers.get("x-request-id"), /^[0-9a-f-]{36}$/i);
    assert.equal(response.headers.get("set-cookie"), null);
    assert.equal(response.headers.get("access-control-allow-origin"), ORIGIN);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("origin must be HTTPS and must not contain a path or credentials", async () => {
  for (const invalid of ["http://orbit-api.example.test", "https://user:pass@api.example.test", "https://api.example.test/base"]) {
    const response = await gateway.fetch(
      new Request("https://orbit-runtime.workers.dev/api/v1/meta"),
      env({ ORBIT_API_ORIGIN: invalid })
    );
    assert.equal(response.status, 503);
  }
});

test("mutating API requests stream their body and preserve the user access token", async () => {
  const originalFetch = globalThis.fetch;
  let capturedBody = "";
  let capturedHeaders;
  globalThis.fetch = async request => {
    capturedHeaders = request.headers;
    capturedBody = await request.text();
    return new Response(JSON.stringify({ data: { id: "incident-1" } }), {
      status: 201,
      headers: { "content-type": "application/json" }
    });
  };

  try {
    const payload = JSON.stringify({ title: "Gateway smoke", severity: "low" });
    const response = await gateway.fetch(new Request("https://orbit-runtime.workers.dev/api/v1/workspaces/00000000-0000-4000-8000-000000000001/incidents", {
      method: "POST",
      headers: {
        origin: ORIGIN,
        authorization: "Bearer operator-access-token",
        "content-type": "application/json"
      },
      body: payload
    }), env());

    assert.equal(response.status, 201);
    assert.equal(capturedBody, payload);
    assert.equal(capturedHeaders.get("authorization"), "Bearer operator-access-token");
    assert.equal(capturedHeaders.get("origin"), ORIGIN);
    assert.equal(capturedHeaders.get("x-orbit-gateway-secret"), SECRET);
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("requests without an Origin header do not receive a permissive CORS grant", async () => {
  const response = await gateway.fetch(
    new Request("https://orbit-runtime.workers.dev/health/live"),
    env()
  );
  assert.equal(response.status, 200);
  assert.equal(response.headers.get("access-control-allow-origin"), null);
});

test("readiness is proxied through the protected origin rather than reported locally", async () => {
  const originalFetch = globalThis.fetch;
  let target;
  globalThis.fetch = async request => {
    target = request;
    return new Response(JSON.stringify({ status: "ready" }), {
      status: 200,
      headers: { "content-type": "application/json" }
    });
  };
  try {
    const response = await gateway.fetch(
      new Request("https://orbit-runtime.workers.dev/health/ready", { headers: { origin: ORIGIN } }),
      env()
    );
    assert.equal(response.status, 200);
    assert.equal(new URL(target.url).pathname, "/health/ready");
    assert.equal(target.headers.get("x-orbit-gateway-secret"), SECRET);
    assert.equal(response.headers.get("cache-control"), "no-store");
  } finally {
    globalThis.fetch = originalFetch;
  }
});

test("authenticated API requests explicitly opt out of upstream caches", async () => {
  const originalFetch = globalThis.fetch;
  let upstreamHeaders;
  globalThis.fetch = async request => {
    upstreamHeaders = request.headers;
    return new Response("{}", { status: 200 });
  };
  try {
    await gateway.fetch(
      new Request("https://orbit-runtime.workers.dev/api/v1/meta", { headers: { origin: ORIGIN } }),
      env()
    );
    assert.equal(upstreamHeaders.get("cache-control"), "no-store");
  } finally {
    globalThis.fetch = originalFetch;
  }
});
