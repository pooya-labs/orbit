# ADR 0001 — OIDC access-token validation and API authorization

- **Status:** Accepted; M06 resource-server implementation in progress
- **Date:** 2026-11-01
- **Scope:** ORBIT Go API, future browser client, and M05 state-changing workflows

## Context

The current ORBIT API has no authentication, role enforcement, or workspace isolation. The Go API and its telemetry stores are intended for local development only; Compose binds its ports to loopback, and the public GitHub Pages deployment is frontend-only.

M05 introduces operations that change state (incident transitions, alert configuration, deployment annotations, SLO configuration, and audit events). Those routes must not be exposed publicly before a common server-side identity and authorization layer exists. A frontend-only check is not an authorization boundary.

## Decision

### 1. Use an OIDC/OAuth resource-server model

The API accepts **OAuth 2.0 access tokens** in the HTTP `Authorization: Bearer` header. It does not accept OIDC ID Tokens as API credentials. The configured authorization server must issue access tokens intended for ORBIT's API audience.

Provider integration remains vendor-neutral. The issuer and audience are deployment configuration, not hard-coded provider URLs.

The expected deployment uses OpenID Provider Discovery for the issuer metadata and its advertised `jwks_uri`. The API validates that the discovered issuer exactly matches the configured issuer. It never trusts `jku`, `jwk`, `x5u`, or another key URL supplied by the untrusted token header.

### 2. JWT validation requirements

Before creating an authenticated principal, the API must validate all of the following:

- signature against a current key from the configured issuer's JWKS;
- exact `iss` match;
- `aud` contains the configured ORBIT API audience;
- `exp` is present and not expired;
- `nbf`, when present, is satisfied;
- `iat` is present and not unreasonably in the future;
- `sub` is present and non-empty;
- `kid` matches a key in the issuer's JWKS;
- signing algorithm is in a server-configured allowlist (initially asymmetric algorithms such as RS256 and ES256 only).

Reject `alg=none`, symmetric MAC algorithms, unsupported algorithms, missing/unknown `kid`, malformed claims, expired tokens, and tokens signed by a different issuer. Never derive accepted algorithms solely from the untrusted JWT header. Apply a small bounded clock skew (initial target: 60 seconds).

Limit the bearer token size (initial target: 16 KiB) before parsing. Accept credentials only from the Authorization header, not query parameters or arbitrary cookies. Do not log bearer tokens or complete authorization headers.

Use a maintained JOSE/JWT implementation with explicit signature and claim validation. The repository currently targets Go 1.24. The reviewed candidate is `github.com/lestrrat-go/jwx/v3` pinned to v3.0.12, whose module declares Go 1.24.0 and matches this repository's Go 1.24 toolchain and must not be adopted without an intentional toolchain upgrade. Reuse a long-lived JWKS cache with controlled refresh and key-rotation behavior; do not fetch remote keys for every request.

### 3. Identity and role mapping

The authenticated principal contains:
- issuer (`iss`);
- subject (`sub`);
- normalized role set;
- optional workspace/tenant identifier;
- optional display metadata for UI only.

The stable actor identity is the issuer + subject pair. Email, display name, username, and request-body fields must never be used as the authorization identity.

The initial role model is:
- `viewer`: read service inventory and telemetry;
- `operator`: viewer permissions plus incident acknowledgement/transition, alert acknowledgement/silence, and deployment annotations;
- `administrator`: operator permissions plus workspace membership, role assignment, alert-rule administration, and security configuration.

Roles are read from a configurable claim (initial default: `roles`) and mapped through an explicit allowlist. Missing, malformed, or unknown roles grant no elevated permissions. Do not infer roles from email domains or user names.

A workspace claim is not a substitute for membership validation. When multi-workspace support is introduced, every database read and write must be scoped to the authenticated workspace and the principal must be verified as a member of that workspace. Do not claim workspace isolation while the database schema and repository methods remain global.

### 4. Route policy and public endpoints

Authorization is centralized in middleware/route policy, not copied into individual handlers.

- Public: `/health/live` and the minimal metadata needed for deployment health checks.
- Authenticated viewer: service inventory and bounded telemetry reads.
- Operator: operational mutations such as incident transitions, alert acknowledgements, silences, and deployment annotations.
- Administrator: role/membership changes, workspace settings, alert-rule administration, and security-sensitive configuration.

The readiness endpoint must not disclose credentials, internal topology, or raw dependency errors. Whether it is public or restricted is a deployment policy; its payload remains minimal.

CORS remains an exact-origin allowlist. Authenticated browser requests require the `Authorization` header to be allowed in preflight. Only the methods actually implemented should be enabled. If authentication is later moved to cookies, CSRF protections, secure/HttpOnly/SameSite cookie settings, and an explicit origin policy become mandatory.

### 5. Browser authentication

For the static browser workspace, the intended browser flow is OIDC Authorization Code with PKCE (S256). Do not use the Implicit or Resource Owner Password Credentials grants. Keep short-lived access tokens in memory and send them as Bearer tokens; do not place access or refresh tokens in localStorage or sessionStorage. Refresh-token rotation and logout/revocation behavior must be selected with the eventual identity provider before browser login is shipped.

No provider-specific callback URLs, secrets, client IDs, or tenant settings are committed in this ADR.

### 6. Development and production behavior

- Local development may run without authentication only when `ORBIT_ENVIRONMENT=development` is explicit. An unset environment defaults to `production` and fails closed.
- If issuer/audience configuration is provided in development, authentication is enabled and invalid tokens must fail closed.
- Staging/production must refuse to start unless issuer, audience, and the required validation/role configuration are present and validated.
- There is no general-purpose `AUTH_DISABLED=true` production bypass.
- The local Compose stack remains loopback-bound and explicitly non-production.

### 7. Audit and state changes

Before M05 mutation routes are exposed:
- actor identity must be derived from a verified access token;
- state-changing operations must use server-side role checks;
- audit events must capture actor issuer/subject, workspace, action, entity, request ID, timestamp, and a redacted change summary;
- audit events should be committed in the same database transaction as the state change where feasible;
- retries and duplicate delivery must be idempotent where background work or notifications are involved.

No mutation handler may accept an actor identity, role, or workspace ID as authoritative input from request JSON.

## Implementation sequence

1. Add validated auth configuration and a startup validation test matrix.
2. Add an OIDC discovery/JWKS-backed access-token verifier with a bounded key cache and explicit algorithm allowlist.
3. Add principal context and centralized route policies; protect read APIs with viewer permissions.
4. Add tests for missing/malformed bearer tokens, signature tampering, issuer/audience mismatch, expiry, not-before, future-issued tokens, unknown key IDs, key rotation, malformed roles, and insufficient permissions.
5. Extend exact-origin CORS to preflight Authorization requests and test disallowed origins/methods.
6. Add workspace membership and scoping only alongside an explicit tenant data model.
7. Implement M05 mutation routes only after steps 1–5 and audit identity are merged and green in CI.

## Consequences

- A provider can be changed by configuration rather than rewriting the API.
- The API can be protected before any state-changing M05 endpoint exists.
- A JWT is not trusted merely because it decodes; signature, issuer, audience, time claims, subject, and roles are all validated.
- Full multi-tenant isolation remains a separate schema and repository milestone.
- The public GitHub Pages preview remains a static/demo frontend until a separately hosted authenticated API is deliberately deployed.

## Security references

- OAuth 2.0 Security Best Current Practice — RFC 9700: https://www.rfc-editor.org/rfc/rfc9700.html
- JSON Web Token Best Current Practices — RFC 8725: https://www.rfc-editor.org/rfc/rfc8725.html
- OpenID Connect Core 1.0 — ID Token validation and Authorization Code flow: https://openid.net/specs/openid-connect-core-1_0.html
- JWT access-token profile (when supported by the chosen provider) — RFC 9068: https://www.rfc-editor.org/rfc/rfc9068.html
