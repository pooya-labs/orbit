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
4. [x] Reject producer-supplied reserved tenant attributes at all traversed OTLP attribute scopes and inject the canonical authorized workspace ID for all three signals (PR #27). PostgreSQL-backed integration tests verify cross-workspace and revocation denial for all signals over HTTP/gRPC. PR #36 verifies forged-attribute rejection, and PR #38 is merged with passing CI for mixed-resource batch rejection across all three signals over both transports.
5. [x] Isolate the local Compose telemetry services on an internal Docker network and remove host-published raw OTLP ports (PR #26). PR #56 adds an optional Caddy TLS edge-ingress Compose overlay and CI-verified isolation: only Caddy publishes edge ports; API OTLP and raw Collector listeners remain unpublished; Caddy is isolated from the application and telemetry networks. Real DNS/ACME, host firewall, trusted-proxy validation, internal-hop mTLS assessment, and cloud/Kubernetes network policy remain deployment-specific work.
6. [x] Verify oversized-request rejection for metrics, logs, and traces over HTTP/gRPC (PR #42). Valid-grant, revoked-identity, cross-workspace, forged-attribute, mixed-resource batch, and oversized-request denial coverage is implemented and CI-verified over both transports. PR #39 and PR #40 are merged and CI-passing: workspace query matchers are verified across Prometheus, Loki, and Tempo, and non-members are rejected before backend access.
7. [x] Verify shared rate-limit enforcement across gateway instances and HTTP/gRPC transports (PR #43; merged, CI-passing).
8. [x] Verify concurrent multi-replica shared rate-limit enforcement (PR #46; merged and CI-passing).
9. [x] Verify bounded gateway admission under saturation for OTLP/HTTP and OTLP/gRPC (PR #51; merged and CI-passing). With one admission slot occupied by a blocked export, a second request is rejected before forwarding; the admitted request completes after the slot is released.
10. [ ] Verify end-to-end that a workload authorized for workspace A cannot write data visible under workspace B, even when it supplies B's identifier. Gateway-level denial tests exist; backend-to-query visibility verification remains open.
11. Run end-to-end tests through Prometheus, Loki, Tempo, and the workspace-scoped ORBIT query endpoints.
12. Add broader burst/load, database/Collector outage, timeout, backpressure, revocation-latency, backup/restore, and deployment-network-policy tests before enabling production ingestion.

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


## ADR 0002 PR #48 revocation verification

PR #48 is merged into `main` (squash merge commit `3c5619d380b498a50660415b68175f8266c48a71`). CI run `41352185701` completed successfully: Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test all passed. The PostgreSQL-backed integration test uses four independent gateway/authorizer instances (two HTTP, two gRPC) and verifies that signal-grant revocation and workload-identity revocation are observed by other replicas for metrics, logs, and traces; denied requests do not reach the Collector forwarders.

The test establishes read-after-commit revocation for requests authorized after the revocation transaction commits. It does not establish cancellation of requests that completed authorization before that commit. In-flight revocation ordering/cancellation remains an explicit open design and test item. Production ingestion remains disabled until TLS/network policy, in-flight semantics, broader load testing, real-provider PKCE, and operational runbooks are complete.


## ADR 0002 — PR #50 in-flight revocation contract

PR #50 is merged into `main` (squash merge commit `45680118f44c3cd35f64214072085e6ca1a43de6`). CI run `41352877449` passed Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test.

The authorization decision is linearized when the shared PostgreSQL repository confirms the workload grant. An export that has already passed authorization and entered the Collector forwarder may complete even if identity revocation commits while that export is in flight. A request whose authorization begins after the revocation commit must be denied and must not reach the forwarder. The contract applies to OTLP/HTTP and OTLP/gRPC.

This is not cancellation, rollback, or recall of telemetry already admitted to the Collector. Operators requiring stronger interruption semantics would need a separately designed and tested downstream cancellation/drain mechanism. The in-flight test covers identity revocation; grant/identity revocation latency under broader load and operational failure conditions remains part of the production hardening work. Production ingestion remains disabled until TLS/network policy, load/failure testing, real-provider PKCE, and runbooks are complete.


## ADR 0002 — PR #53 gRPC dependency and timeout verification

PR #53 is merged into `main` (squash merge commit `13cd7b4927b2259b10b5dd94824a70ee11541069`). CI run `41353731713` passed Go API tests/build, web typecheck/production build, and the local telemetry pipeline end-to-end smoke test. Tests verify that an unavailable shared rate-limit store fails closed with gRPC `Unavailable` and no Collector forwarding, ordinary Collector forwarding errors map to `Unavailable`, and a forward context deadline maps to `DeadlineExceeded`.

PR #54 is merged with passing CI and makes the corresponding OTLP/HTTP deadline contract explicit as HTTP 504 Gateway Timeout. HTTP and gRPC now have explicit timeout status coverage. These deterministic tests do not establish complete production database/Collector outage, saturation, or recovery behavior.


## ADR 0002 — PR #56 optional TLS edge ingress

PR #56 is merged into `main` (squash merge commit `45997fd2ec508e260a51a027208152c9b155ac0f`). CI run `41354509326` passed Go API tests/build, web typecheck/production build, and the local telemetry pipeline smoke test. The new optional Compose overlay places Caddy on a dedicated ingress network, routes separate OTLP/HTTP and OTLP/gRPC hostnames to the authenticated ORBIT listeners, and publishes only the edge ports 80/443. API OTLP ports 4319/4320 and raw Collector ports 4317/4318 remain unpublished. Caddy is not attached to the application default or telemetry-internal networks.

This establishes the reviewed Compose topology and Caddy configuration; it does not prove a real internet-facing deployment. DNS/ACME issuance, host firewall rules, trusted-proxy behavior, production secret delivery, internal-hop mTLS assessment, and cloud/Kubernetes NetworkPolicy remain open. The edge proxy does not replace workload JWT verification, workspace grants, signal permissions, or shared rate limiting. Production tenant ingestion remains disabled.


## ADR 0002 — PR #58 dependency recovery coverage

PR #58 is merged into `main` (squash merge commit `fd7c7d23d952bdb6f08250c2dcd699c2816620d3`). CI run `41355041031` passed all three jobs. HTTP and gRPC tests inject transient errors from the workload grant repository, shared rate-limit store, and Collector forwarder. Database dependency failures fail closed and are not forwarded; a later request succeeds through the same gateway instance after the dependency error is cleared.

This provides deterministic recovery-contract coverage, not real process/network fault testing. PostgreSQL/Collector restart behavior, network partitions, recovery under sustained load, and explicit backpressure remain open before production tenant ingestion can be enabled.


## ADR 0002 — PR #60 default burst-capacity verification

PR #60 is merged into `main` (squash merge commit `cb85c0afea6647ecc59b9fe59e499ce8383fc526`). CI run `41355534531` passed Go race tests/build, web typecheck/build, and the local telemetry pipeline smoke test. Tests fill all 64 default HTTP/gRPC admission slots using blocking Collector forwarders, verify additional requests fail with the transport-specific capacity status before forwarding, then release all admitted requests and verify they complete with no capacity leak.

The tests prove the configured in-process cap and deterministic rejection behavior, not sustained throughput, real infrastructure fault recovery, or backpressure behavior at production load.


## ADR 0002 — PR #62 real Collector listener recovery

PR #62 is merged into `main` (squash merge commit `534f6d02c04616a5d27831460b24fd87db8bfade`). CI run `41357122909` passed Go race tests/build, web typecheck/production build, and the telemetry end-to-end smoke test.

HTTP and gRPC recovery tests now use real local TCP Collector listeners with the production forwarder implementations. Each test establishes a successful export, stops the Collector listener, verifies the live gateway cannot export while the listener is down, restarts the listener on the same address, and verifies the same gateway and forwarder succeed after reconnect.

This establishes listener stop/restart recovery in-process. It does not establish recovery from container orchestration, packet loss/network partitions, PostgreSQL process restarts, or sustained production load. These remain open validation gates; production tenant ingestion remains disabled.


## ADR 0002 — PR #64 PostgreSQL pool recovery after network interruption

PR #64 is merged into `main` (squash merge commit `6b2e3418be2e8fdff0c9634b361acaa5bb6d6da1`). CI run `41357596656` passed the complete pipeline. The integration test opens the production PostgreSQL Repository/pgxpool through a local TCP proxy, warms a connection, closes established tunnels and rejects new connections, verifies Ping fails while the database network path is unavailable, then restores the proxy and verifies the same pool reconnects without recreating the Repository.

The database server itself remains online. This establishes recovery from a TCP path interruption, not PostgreSQL process restart, container replacement, or packet-loss/partition tolerance. Those and sustained-load/backpressure tests remain open validation work.
