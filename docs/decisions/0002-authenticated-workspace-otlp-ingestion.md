# ADR 0002 — Authenticated workspace-aware OTLP ingestion

- Status: Accepted (implementation in progress; production acceptance criteria remain open)
- Date: 2026-11-01
- Depends on: ADR 0001 (OIDC access-token authentication), workspace membership and telemetry query scoping
- Scope: M06 production hardening

## Context

The local OpenTelemetry Collector accepts OTLP over gRPC and HTTP without authentication. The Compose observability profile now keeps the collector and telemetry backends on an internal Docker network and does not publish raw OTLP receiver ports to the host. This is a local-development network boundary, not an authenticated production ingestion gateway.

Workspace-scoped query endpoints require the canonical `orbit_workspace_id` Prometheus label, Loki label, and `orbit.workspace.id` Tempo resource attribute. A query selector alone does not prove that the producer was entitled to write data under that workspace. An OTLP producer can submit arbitrary resource attributes, including a forged workspace identifier. Trusting that value would let one tenant inject telemetry into another tenant's namespace.

The existing OIDC verifier authenticates human API access tokens. Workspace membership is a separate authorization relation and must not be inferred from a user-supplied workspace ID, email, display name, or an unvalidated token claim.

## Decision

Do not expose the Collector's raw OTLP receivers as a public multi-tenant endpoint. Introduce a dedicated authenticated ingestion boundary in front of the private Collector.

### Identity and authorization

1. Workloads authenticate with short-lived OAuth 2.0 access tokens issued for a dedicated ORBIT telemetry-ingestion audience. Human dashboard tokens and ID tokens are not ingestion credentials.
2. The gateway validates signature, issuer, audience, expiry, not-before, issued-at, subject, token type, and allowed signing algorithm using the configured issuer's trusted JWKS. Validation fails closed.
3. The gateway resolves the verified `(issuer, subject)` pair to a registered workload/service identity in PostgreSQL. That record grants one or more explicit workspace bindings and permitted signal types. A workspace ID in an incoming OTLP attribute or arbitrary request header is never authoritative.
4. The gateway rejects unknown, disabled, expired, or revoked workload identities and rejects a workspace binding that is not explicitly granted to that identity.
5. Credential rotation and revocation must be supported. Secrets and bearer tokens must never be logged; API responses and audit events must not reveal credential material.

### Canonical tenant attribution

1. The gateway removes or rejects producer-supplied `orbit.workspace.id`, `orbit_workspace_id`, and equivalent reserved tenant attributes before forwarding.
2. After authorization, the gateway writes the canonical workspace identifier from the server-side workload binding to every resource/signal batch.
3. If a batch contains mixed or ambiguous tenant identity, reject the whole batch. Do not partially forward it.
4. Prometheus, Loki, and Tempo receive only canonical tenant attribution from the trusted gateway. Collector processors must not convert an untrusted producer value into a tenant label.
5. The Collector and backend write endpoints remain private-network-only. Network policy/firewall rules must prevent clients from bypassing the gateway and reaching raw receivers or storage APIs.

### Gateway listener and TLS boundary

The application HTTP and gRPC ingestion listeners do not terminate TLS. Their default bind addresses are loopback-only (`127.0.0.1:4319` and `127.0.0.1:4320`). A deployment that needs remote workload connectivity must explicitly bind them to a private interface and place them behind a TLS-terminating ingress.

Before enabling remote traffic, the deployment must:
- terminate TLS at the ingress and reject plaintext external connections;
- forward the original Authorization metadata/header unchanged and never log bearer credentials;
- restrict listener reachability to the ingress using security groups, firewall rules, or Kubernetes NetworkPolicy;
- keep Collector OTLP receivers and telemetry backend write APIs on a separate private network, reachable only by the gateway;
- avoid publishing gateway or raw Collector ports directly to the public internet.

The local Compose profile binds the gateway listeners to the container network only and publishes neither gateway nor raw Collector OTLP ports to the host. This is not a substitute for production ingress TLS and network-policy verification.

### OTLP protocol boundary

- Support OTLP/HTTP and OTLP/gRPC only through the authenticated gateway.
- Enforce request/body limits, per-credential and per-workspace rate limits, bounded concurrent requests admitted before token verification/database work, timeouts, and backpressure. The gRPC server also caps concurrent streams per connection.
- Return protocol-appropriate authentication/authorization errors; do not leak whether another workspace or credential exists.
- Propagate request/correlation identifiers without accepting them as identity.
- Keep the existing loopback-only unauthenticated Compose profile explicitly development-only. It must not be reused as a production deployment template.

### Data model and audit

The implementation must add a workload identity model distinct from human workspace membership. At minimum it needs:
- stable identity ID and unique issuer/subject pair;
- active/revoked state and credential lifecycle metadata;
- explicit workspace grants and allowed telemetry signals;
- created/updated/revoked timestamps and actor attribution;
- indexed lookup by issuer/subject and workspace;
- audit events for creation, grant/revoke, rotation, disablement, and rejected authorization decisions (without secrets).

Workspace grants must be checked server-side on every ingestion authorization decision or through a cache with bounded lifetime and explicit revocation invalidation. A stale authorization cache must not permit indefinite access after revocation.

## Threats and required mitigations

| Threat | Required mitigation |
| --- | --- |
| Producer forges another workspace ID | Reject producer-supplied reserved attributes and inject the DB-authorized canonical ID |
| Token replay or theft | Short token lifetime, TLS, scoped audience, rotation/revocation, rate limits |
| Credential from one service writes another signal/workspace | Explicit per-identity workspace and signal grants |
| Direct Collector bypass | Private bind/network policy; backend and receiver ports not publicly exposed |
| Cross-tenant query after ingestion | Existing workspace membership checks plus mandatory tenant selectors in all three backends |
| Cardinality/cost abuse | Attribute allowlist, request/body limits, per-workspace quotas, retention controls |
| Authorization database unavailable | Fail closed; never fall back to producer-supplied workspace attributes |
| Revoked identity remains cached | Bounded cache TTL and invalidation; document and test revocation latency |

## Implementation sequence

1. [x] Add workload identity, workspace grant, and audit schema/repository methods with transactional grant changes (PR #21). Rejected authorization decisions are now appended to the audit log without storing credentials or telemetry payloads (PR #23).
2. [x] Add an ingestion-specific verifier configuration with a dedicated audience; do not weaken or repurpose the human API audience (strict RFC 9068 `at+jwt` profile and explicit `nbf` requirement merged in PR #22).
3. [x] Implement OTLP/HTTP and OTLP/gRPC transport handlers with per-request workload verification and workspace/signal grant checks (PRs #28 and #29); dedicated runtime listeners and a separate strict workload verifier are wired in `server/cmd/orbit-api`.
4. [x] Reject producer-supplied reserved tenant attributes at all traversed OTLP attribute scopes and inject the canonical authorized workspace ID for all three signals (PR #27). PostgreSQL-backed integration tests verify cross-workspace and revocation denial for all signals over HTTP/gRPC. PR #36 is merged and CI-verified; forged-attribute tests cover all three signals over both transports. Mixed-batch coverage remains open.
5. [x] Isolate the local Compose telemetry services on an internal Docker network and remove host-published raw OTLP ports (PR #26). Production gateway-to-collector network policy and deployment firewall rules remain outstanding.
6. [ ] Complete the remaining mixed-batch and oversized-request integration tests for metrics, logs, and traces. Valid-grant, revoked-identity, cross-workspace, and forged-attribute denial coverage is implemented and CI-verified over both transports.
7. [ ] Verify end-to-end that a workload authorized for workspace A cannot write data visible under workspace B, even when it supplies B's identifier. Gateway-level denial tests exist; backend-to-query visibility verification remains open.
8. Run end-to-end tests through Prometheus, Loki, Tempo, and the workspace-scoped ORBIT query endpoints.
9. Add load, rate-limit, revocation-latency, backup/restore, and deployment-network-policy tests before enabling production ingestion.

## Acceptance criteria

- No public unauthenticated OTLP receiver exists in a production deployment.
- All accepted telemetry is attributed to a workspace resolved from a verified workload identity and server-side grant.
- Producer-supplied reserved workspace attributes cannot override or spoof the canonical workspace.
- Cross-workspace ingestion and query isolation is proven for metrics, logs, and traces.
- Revoked/disabled identities fail closed within the documented revocation bound.
- CI validates OTLP/HTTP and OTLP/gRPC authorization and end-to-end tenant isolation.
- Runbooks cover credential issuance, rotation, revocation, incident response, and Collector/backend network isolation.

## Consequences

This introduces a distinct machine identity and authorization lifecycle; human OIDC roles and workspace membership are not reused as a shortcut. The gateway is a security-critical component and must remain small, auditable, bounded, and independently tested. Until all acceptance criteria pass, tenant telemetry ingestion remains disabled for production and M06 remains incomplete.
