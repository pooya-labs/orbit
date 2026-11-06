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
    Object.defineProperty(request, "cf", { value: { country: "BE" } });
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
