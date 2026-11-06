import test from "node:test";
import worker from "./worker.mjs";
import assert from "node:assert/strict";
import {
  incidentTransitionAllowed,
  parseBearer,
  roleAtLeast,
  validIssuer,
  validWorkspaceSlug
} from "./worker.mjs";

test("workspace slugs follow the API contract", () => {
  for (const value of ["a", "orbit", "pooya-labs-2"]) assert.equal(validWorkspaceSlug(value), true);
  for (const value of ["", "-orbit", "Orbit", "orbit-", "two words", "x".repeat(49)]) assert.equal(validWorkspaceSlug(value), false);
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
