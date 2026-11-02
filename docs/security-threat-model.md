# ORBIT security threat model

- Status: Working baseline; **not a production security sign-off**
- Scope: The current ORBIT web workspace, Go API, PostgreSQL store, authenticated OTLP gateways, local Collector/backends, GitHub Actions, and documented production-ingress scaffold.
- Last reviewed: 2026-11-02
- Related decisions: ADR 0001 — OIDC access-token authentication; ADR 0002 — authenticated workspace OTLP ingestion.

This document records the trust boundaries that exist today, evidence that supports them, and residual risks that still block production tenant traffic. It is deliberately conservative: a CI integration test or a local Compose network is not treated as proof of a production cloud deployment.

## 1. System and security objectives

ORBIT is an observability control plane. The raw telemetry stores remain Prometheus, Loki, and Tempo; PostgreSQL stores workspace identity, membership, workload grants, service/catalog metadata, operator workflows, and durable histories.

The main security objectives are:

1. A user or workload must prove its identity before protected data access or ingestion.
2. Every tenant-facing read or write must be authorized against server-side workspace membership or workload grants.
3. Producer-controlled telemetry attributes must never select the tenant namespace.
4. Tokens, passwords, webhook URLs, and webhook credentials must not be committed, returned to browsers, or written to logs.
5. Operator history must remain append-only and attributable without exposing raw identity subjects to the browser.
6. Recovery controls must be tested without implying that local CI proves a production backup or recovery service.

## 2. Assets and trust boundaries

| Asset | Security property | Primary trust boundary |
| --- | --- | --- |
| Human OIDC access tokens and browser session state | Short-lived, audience-scoped; not persisted as long-lived browser secrets | Browser ↔ API ↔ configured OIDC issuer |
| Workload access tokens | Dedicated ingestion audience and at+jwt profile; bearer material is not logged | Workload ↔ authenticated OTLP gateway |
| Workspace membership and workload grants | PostgreSQL is authoritative; producer-supplied workspace IDs are not | API/gateway ↔ PostgreSQL |
| Metrics, logs, and traces | Canonically attributed to the authorized workspace and isolated at query time | Gateway ↔ private Collector ↔ telemetry backends ↔ scoped API |
| Operator histories and audit events | Append-only; actor identity represented safely in browser read models | API transaction ↔ PostgreSQL ↔ authenticated workspace UI |
| Webhook endpoint credentials | Deployment-managed secret references only; exact HTTPS host allowlist | Notification worker ↔ secret environment/provider |
| Database dumps and recovery credentials | Confidential, access-controlled, restorable, and kept outside Git | PostgreSQL ↔ backup storage/restore environment |
| Source, dependencies, and CI credentials | Changes reviewed; no hard-coded credentials; least-privilege workflow tokens | Pull request/source tree ↔ GitHub Actions and third-party actions |

## 3. Threats, current controls, and residual risk

| Threat scenario | Controls implemented and evidence | Residual risk / required action |
| --- | --- | --- |
| Forged, expired, or wrong-audience human token | Server-side OIDC discovery/JWKS verification, issuer/audience/algorithm/time/subject checks, centralized viewer/operator/administrator route policy; see ADR 0001 and Go verifier tests | A real production identity provider, browser callback, audience, CORS, logout, and key/token lifecycle are not configured or verified for the public site |
| Human token reused as a workload credential | Workload verifier is separate, requires dedicated audience, typ=at+jwt, and nbf; startup enforces different human/workload audiences | Provider-issued workload credentials, automated issuance/rotation, and production TLS delivery are deployment gates |
| Workload writes to a workspace it does not own | Gateway resolves verified issuer/subject to PostgreSQL workload identity and explicit per-workspace/per-signal grants on each authorization decision; unknown, disabled, revoked, or ungranted identities fail closed | The local CI identity verifier is deterministic in backend round-trip tests; production issuer and key rotation must be verified independently |
| Producer spoofs another tenant in resource attributes | Reserved tenant attributes are rejected at traversed OTLP scopes; the gateway injects canonical tenant attribution; mixed-resource batches are rejected | Deployment must prevent direct access to raw Collector receivers and backend write APIs; cloud/Kubernetes network policy is not supplied by the Compose scaffold |
| Workspace A data is queried as workspace B | Workspace API verifies membership before backend access and applies mandatory workspace matchers. PR #84 adds real PostgreSQL → gateway → Collector → Prometheus/Loki/Tempo → workspace API A-vs-B tests for metrics/logs/traces (CI run 41383068747) | The proof is the local Compose backend path. Production managed stores, per-workspace quotas, remote networking, and deployment-specific isolation must be tested in the target environment |
| Client bypasses the authenticated gateway | Local raw Collector receivers and backend APIs are attached only to the private telemetry network and are not host-published. Optional Caddy scaffold publishes only edge ports | Caddy is a scaffold, not a live ingress. Real DNS/ACME, host firewall, trusted-proxy behavior, mTLS assessment, security groups, and cloud/Kubernetes NetworkPolicy remain open |
| Revoked workload continues to export | PostgreSQL-backed grant/identity checks are observed across independent HTTP/gRPC gateways. The PR #50 contract linearizes authorization at the grant check | An export already authorized and admitted to the Collector may complete after the revoke transaction commits. This is not downstream cancellation; target-environment revocation latency still needs measurement |
| Ingestion overload or cardinality/cost abuse | Shared PostgreSQL rate limits, bounded request sizes, admission before expensive verification/DB work, 64-slot default capacity, explicit fail-closed error handling, and CI-checked local metrics/log/trace retention and size limits | In-process benchmarks do not establish real multi-process capacity or production p95/p99. Local backend limits are single-tenant defaults, not production per-workspace quotas |
| Webhook SSRF or credential disclosure | HTTPS/port-443 allowlist, exact host checks, public-unicast DNS validation, no redirects, bounded timeouts, and secret_ref rather than raw URL/credential in API/storage | Production secret-manager integration, credential rotation/expiry, adapter-specific incident response, and delivery monitoring remain deployment work |
| Audit history alteration or misleading attribution | PostgreSQL append-only guards protect audit events and actor mappings; migration 015 records workspace-local pseudonymous actor refs and role-at-event snapshots. Historical roles remain unknown when they cannot be reconstructed safely | Raw OIDC issuer/subject values remain server-side. A retention/archive/compliance policy for durable history has not been approved; history must not be TTL-pruned before that policy exists |
| Database loss or corrupted recovery | CI runs a real PostgreSQL 17 pg_dump/pg_restore round-trip into a separate database and verifies migration ledger, workspace/audit data, actor context, and append-only guards (PR #83) | No production encrypted off-host schedule, PITR/WAL archiving, retention policy, restore rehearsal, backup alerting, or measured RPO/RTO is configured |
| Secret or dependency compromise in source/CI | The Security workflow runs full-history Gitleaks, govulncheck, npm audit, and PR Dependency Review; Dependabot proposes dependency/action updates. The action-pin policy requires full commit SHAs with version annotations. PR #90 scans the production API and Compose/ingress images with the pinned Trivy Action v0.36.0 and Trivy v0.76.1, failing on high/critical findings. The PostgreSQL helper is rebuilt from pinned gosu source using a patched Go toolchain; the telemetry smoke runner is a static standard-library-only Go binary; Tempo is rebuilt with patched go-jose; and the ORBIT Collector is reduced to a minimal custom distribution to avoid shipping unrelated contrib components. | Gitleaks, code-path Go vulnerability analysis, npm audit, and immutable action-pin checks have run. The rebuilt PostgreSQL and API images and the minimal custom Collector have passed their current image scans. Tempo, Caddy, and remaining image scans must pass on the latest PR head before the image gate is complete. Dependency Review cannot run until GitHub Dependency Graph is enabled. Findings still require triage/remediation; credential rotation, remediation ownership/SLAs, and formal release provenance/SBOM remain open |

## 4. Security invariants for future changes

- Never infer tenant identity from an OTLP attribute, HTTP header, service name, email, browser query parameter, or unvalidated token claim.
- Never add an unscoped fallback to workspace-scoped Prometheus, Loki, or Tempo queries.
- Never publish raw Collector receivers (4317/4318), gateway listeners (4319/4320), or backend query/write ports to the public host.
- Never log Authorization headers, access/refresh tokens, webhook URLs, secret values, database URLs, or complete telemetry payloads.
- Never store webhook endpoint credentials in PostgreSQL or accept them from the browser; retain only deployment-managed secret references.
- Keep durable audit, incident, alert, notification, and deployment histories append-only. Do not add TTL deletion without an approved retention/archive/compliance policy.
- Fail closed when OIDC verification, workload grants, PostgreSQL authorization, or rate-limit state is unavailable.
- Any new state-changing API operation must be workspace-scoped, role-checked, transactional with its audit record where possible, and covered by integration tests.
- Any new dependency or GitHub Action must have a clear owner, update path, and security review. Prefer maintained actions with least privileges; every external workflow action must be pinned to a full commit SHA and carry a human-readable version comment. CI enforces this policy; Dependabot proposes the corresponding updates.

## 5. Production go-live gates

Production tenant ingestion remains disabled until all relevant target-environment gates are verified and recorded:

- Real human and workload OIDC providers, callback/CORS/audience, token issuance, key rotation, revocation, and logout.
- TLS from workload ingress to the gateway; trusted proxy and host-firewall rules; no public Collector/backend access; cloud/Kubernetes network policy; and an explicit internal-hop mTLS decision.
- Managed-backend retention, cardinality, ingestion, and per-workspace cost budgets.
- Sustained ingestion and dashboard-query load with real PostgreSQL and Collector dependencies; p50/p95/p99 latency, rejection rates, saturation, backpressure, dependency outages, and recovery.
- Encrypted off-host PostgreSQL backups, an approved retention/PITR/WAL policy, alerting, a rehearsed managed-database restore, and measured RPO/RTO.
- Credential/secret rotation runbooks, incident response, operator access review, dependency remediation ownership, and deployment monitoring.

The deployment and operations procedure baseline is documented in [the operator runbook](operations-runbook.md), but environment-specific execution evidence remains required. The automated checks in GitHub Actions are regression gates for the repository's code and local Compose topology; Dependency Review additionally requires the repository Dependency Graph setting to be enabled. These checks are not a substitute for environment-specific validations.
