# ORBIT deployment and operations runbook

- Status: operator procedure baseline; **not a production-readiness approval**
- Last reviewed: 2026-11-02
- Scope: current ORBIT web workspace, Go API, PostgreSQL, authenticated OTLP gateways, optional Collector/backends, and the optional Caddy edge-ingress Compose overlay.

This runbook makes deployment and incident work repeatable. It does not claim that ORBIT has a configured production provider, public OTLP domains, managed database, encrypted backup service, or production capacity certification. Production tenant ingestion must remain disabled until every applicable gate below has recorded evidence.

## 1. Operating boundaries

- The public GitHub Pages site is a static dashboard. It does not host the Go API or telemetry backends. Demo mode is illustrative; API mode requires a separately hosted API.
- The default Compose stack and the optional observability profile are local-development facilities. Their credentials, open local ports, single-tenant telemetry limits, and local backend configuration are not production defaults.
- \`deploy/production/compose.ingress.yaml\` is an optional Caddy TLS edge scaffold. It is not a deployed ingress, identity provider, cloud firewall, Kubernetes NetworkPolicy, managed quota system, or secret manager.
- The API's human OIDC and workload OIDC audiences must be distinct. The server must fail closed when identity verification, workspace grants, PostgreSQL authorization, or shared rate-limit state is unavailable.
- Tenant identity comes from verified server-side workload grants. Never trust a producer-supplied workspace attribute or add an unscoped query fallback.
- Durable workspace audit events, incident timelines, alert lifecycle events, notification attempts, and deployment annotations are append-only. Do not TTL-delete them until an approved retention/archive/compliance policy exists.

## 2. Release record and ownership

Every non-local deployment should have a change record before rollout. Record:

| Field | Required evidence |
| --- | --- |
| Change owner and approver | Named operational owners and approval reference |
| Release | ORBIT version, immutable commit SHA, and artifact/image digest where applicable |
| CI | Links to the exact commit's API, web, telemetry, backup/restore, and security workflow runs |
| Configuration | Secret-free configuration revision and deployment environment |
| Identity | Human/workload issuer and audience identifiers (never tokens or private keys) |
| Network | DNS names, certificate status, firewall/security-group review, and exposed-port scan |
| Data protection | Backup identifier/time, encryption/key reference, retention policy, and last successful restore drill |
| Capacity | Target-environment load-test report, dependency topology, limits, and saturation/recovery results |
| Recovery | Previous known-good release, database compatibility assessment, rollback owner, and escalation path |

Do not put bearer tokens, private keys, webhook URLs, secret values, database URLs, populated environment files, or raw customer telemetry in the record.

## 3. Production preflight gates

Treat each item as a blocking gate. A local CI result is evidence for code behavior, not proof of a target deployment.

### Identity and authorization

- [ ] Register and configure the real human OIDC provider, public client, callback URL, CORS policy, issuer, and API audience.
- [ ] Configure the separate workload OIDC issuer and audience; verify real issuer discovery, JWKS rotation, token type, expiry, not-before, subject, and audience behavior.
- [ ] Exercise sign-in, refresh (when supported), sign-out, key rotation, revoked workload grants, and revoked identities in the target environment.
- [ ] Confirm that workspace membership is checked before every tenant-scoped read and that every workload has only the required workspace/signal grants.
- [ ] Confirm that no default/demo identity or unauthenticated production bypass is enabled.

### Network and ingress

- [ ] Point the dedicated OTLP/HTTP and OTLP/gRPC DNS names at the intended ingress and verify public DNS from outside the deployment network.
- [ ] Verify ACME issuance and renewal for both names and test the certificate chain from an external client.
- [ ] Restrict host firewall/security groups to TCP 80/443 and optional UDP 443 only where needed.
- [ ] Keep API gateway listeners 4319/4320, raw Collector receivers 4317/4318, PostgreSQL, Redis, NATS, Prometheus, Loki, and Tempo private and unpublished.
- [ ] Verify the real trusted-proxy configuration and client-IP handling. Assess whether the internal proxy-to-API hop requires mTLS.
- [ ] Apply and test cloud/Kubernetes network policy so only intended services can reach the gateway, Collector, database, and telemetry backends.
- [ ] Confirm that public clients cannot write directly to the Collector or query/write directly to any telemetry backend.

### Persistence, secrets, and abuse controls

- [ ] Use managed PostgreSQL or an explicitly hardened PostgreSQL deployment with TLS, least-privilege roles, monitored storage, tested connection limits, and a documented failover model.
- [ ] Provision credentials through a secret manager. Rotate database, OIDC, webhook, and encryption credentials without committing or logging their values.
- [ ] Configure per-workspace managed-backend quotas and retention/cardinality budgets. Local Compose limits are not tenant quotas.
- [ ] Verify shared rate limits, request-size limits, gateway admission bounds, and overload behavior using the target topology.
- [ ] Scan the exact release image and base layers; review dependency and secret-scan findings. Pin third-party GitHub Actions to immutable commit SHAs under the repository action-update policy.
- [ ] Approve the retention/archive policy for durable operator history before implementing any deletion or archival job.

### Backup and recovery

- [ ] Configure encrypted off-host full-database backups, access logging, least-privilege access, key rotation, and a deployment-approved retention schedule.
- [ ] Decide whether PITR/WAL archiving is required or explicitly accept snapshot-only recovery; document the target RPO and RTO.
- [ ] Alert on missing, failed, late, or unverifiable backups.
- [ ] Restore a recent production-like backup into a separate database using the intended PostgreSQL version and topology. Verify migrations, representative workspace reads, audit history, actor context, and append-only guards.
- [ ] Record the restore duration, measured RPO/RTO, data-integrity checks, operator, and follow-up actions.

### Capacity and readiness

- [ ] Run sustained multi-replica ingestion and dashboard-query tests with real PostgreSQL and Collector dependencies.
- [ ] Record throughput, p50/p95/p99 latency, errors/rejections, database pool pressure, Collector queue/memory behavior, backend saturation, and recovery after dependency faults.
- [ ] Verify explicit backpressure and bounded behavior during sustained dependency degradation.
- [ ] Define alert thresholds, escalation ownership, maintenance windows, and a support/on-call route.

A gate is complete only when its evidence is linked in the release record. If any required gate is unknown or failed, do not enable production tenant ingestion.

## 4. Safe deployment procedure

1. Select a reviewed commit and artifact. Deploy the exact same artifact to staging first; do not deploy a moving branch name or an unreviewed build.
2. Confirm that the release record exists and all code-level checks for that commit passed. Triage high/critical dependency or secret findings rather than adding blanket allowlists.
3. Take and verify a pre-deployment database backup. Confirm restore ownership and the rollback target.
4. Review the migration set and application compatibility. Back up before schema changes. Treat migrations as forward-only unless a specific, tested down-migration exists; never improvise destructive SQL during a rollback.
5. Inject production-only configuration from the secret manager. Do not reuse local Compose credentials or commit a populated \`.env\` file.
6. Validate the merged Compose topology before starting the optional ingress scaffold:

   \`\`\`sh
   docker compose \\
     -f docker-compose.yml \\
     -f deploy/production/compose.ingress.yaml \\
     --profile observability \\
     --profile production-ingress \\
     config --quiet
   \`\`\`

7. Start or update services only through the deployment's approved procedure. The reference command in [the ingress guide](production-ingress.md) is illustrative; replace every local-only dependency and credential with the approved target environment before use.
8. Observe startup, migration, readiness, and ingress logs. Do not expose tenant ingestion while any dependency is degraded or any security gate is unresolved.

## 5. Post-deployment verification

Record the result of each check against the deployed commit:

- [ ] \`/health/live\` reports process liveness and \`/health/ready\` confirms required database readiness.
- [ ] \`/api/v1/meta\` reports the expected application version/build identity.
- [ ] The dashboard loads from its intended origin. API mode points only to the explicitly configured HTTPS API; demo mode remains visibly labelled.
- [ ] Human OIDC login, callback, audience validation, workspace selection, and role-based controls work with the real provider.
- [ ] OTLP/HTTP and OTLP/gRPC clients can connect through their respective TLS names; expired, malformed, wrong-audience, ungranted, and revoked credentials fail closed.
- [ ] An authorized test workload sends metrics, logs, and traces. Workspace A can query its own test data; workspace B cannot see it. A direct write to B and forged tenant attributes are rejected.
- [ ] Prometheus, Loki, and Tempo are reachable only through intended private paths and the bounded ORBIT API query routes.
- [ ] A test alert follows the expected lifecycle and a test webhook delivery is visible in immutable attempt history. No endpoint URL or credential appears in the browser or logs.
- [ ] Audit history records the expected actor reference and captured role without exposing raw OIDC issuer/subject values.
- [ ] Backup monitoring, resource/queue saturation alerts, certificate monitoring, and incident escalation are active.

If a tenant-isolation check fails, stop tenant ingestion immediately and follow the security incident procedure below. Do not treat a successful health endpoint as proof of tenant isolation.

## 6. Routine operations

### Each deployment

- Compare the deployed commit and artifact digest with the approved release record.
- Review migrations, dependency findings, changed network exposure, secret references, and rollback compatibility.
- Verify the post-deployment checks in section 5 and attach their evidence.

### Daily or on each staffed operating day

- Review API availability/readiness, error rates, request latency, PostgreSQL connection/pool pressure, disk headroom, and service restarts.
- Review Collector export failures, queue pressure, memory usage, scrape gaps, and Prometheus/Loki/Tempo ingestion/query health.
- Review failed or repeatedly retried webhook deliveries and check that disabled channels remain disabled.
- Verify the latest backup completed and is readable; investigate missing backup evidence immediately.
- Review security/dependency alerts and ensure findings have an owner and due date.

### Periodically

- Perform a restore rehearsal and update measured RPO/RTO.
- Exercise credential/key rotation and workload-grant revocation.
- Re-run sustained-load and recovery tests after meaningful topology, limit, backend, or dependency changes.
- Review operator access, workspace membership, webhook secret references, retention policy, and runbook ownership.
- Review certificate renewal and external exposure from outside the deployment network.

## 7. Incident response

For every incident: assign an incident lead, record UTC timestamps and affected release/configuration, preserve relevant logs and audit evidence, communicate impact, and keep secrets and raw sensitive payloads out of tickets. Prefer reversible containment. Do not delete immutable histories.

| Symptom | Immediate checks and containment | Recovery evidence |
| --- | --- | --- |
| API or database unavailable | Check readiness, PostgreSQL health, connection saturation, migration/startup logs, and the shared rate-limit dependency. Keep authorization fail-closed; do not bypass it to restore service. | Same release or a reviewed fix becomes ready; workspace reads and migration ledger are verified. |
| Metrics/logs/traces stop arriving | Check DNS/TLS, gateway auth errors, grants, rate-limit rejection, admission saturation, Collector availability/queues, and backend health. Do not publish raw Collector ports as a workaround. | A controlled test export is visible through the correct workspace query and not visible to another workspace. |
| Cross-workspace data exposure or forged tenant accepted | Treat as a security incident. Disable tenant ingress at the edge, preserve evidence, revoke affected workload identities/grants, and prevent direct backend access. Do not rely on filtering in the dashboard as containment. | Root cause is fixed; cross-workspace regression tests pass; target-environment A/B isolation is reverified before re-enabling traffic. |
| OIDC key, token, or credential compromise | Disable affected credentials/clients, rotate signing or secret material through the provider/secret manager, review token and audit activity, and revoke affected workload grants. Never paste tokens into incident records. | New credentials validate, old credentials fail, key rotation/revocation behavior is tested, and access is reviewed. |
| Webhook failures or suspicious delivery | Disable the affected workspace channel, preserve delivery-attempt history, inspect destination validation and worker errors, rotate the deployment-managed secret, and check for repeated failures. | Safe destination and secret are confirmed; a controlled delivery succeeds; attempt history remains intact. |
| Backup missing, corrupt, or restore fails | Escalate as a data-protection incident. Preserve available snapshots and logs; do not overwrite the source database during restore testing. | A separate target restore succeeds and integrity plus RPO/RTO evidence is recorded. |

### Security incident stop condition

If workspace authorization, tenant attribution, secret confidentiality, or direct-network isolation cannot be trusted, stop production tenant ingestion first. Do not disable authentication, rate limits, or tenant filters as a temporary workaround. Re-enable only after the incident lead accepts verified remediation evidence and the required target-environment checks pass.

## 8. Rollback and recovery boundaries

- Prefer rolling the application artifact back to the previously approved immutable release only after confirming that the current database schema remains backward-compatible with it.
- Do not automatically roll back database migrations. Do not drop tables, truncate audit/history tables, or rewrite append-only records to make an older binary start.
- If a migration is incompatible, stop writes as required and use a reviewed forward-fix or a separately restored database. Never test restore by overwriting the live source database.
- Keep the previous artifact, configuration revision, secret-version references, and backup identifier available to the recovery owner.
- After rollback, repeat health, authorization, workspace-isolation, and telemetry-query checks. Record the reason, exact commits, data impact, and any events that may need reconciliation.

For PostgreSQL logical backup and restore commands, safety boundaries, and the CI round-trip, follow [the PostgreSQL backup/restore runbook](postgres-backup-restore.md). For public OTLP topology and ingress configuration, follow [the production ingress reference](production-ingress.md).

## 9. Evidence template

Copy this block into the deployment or incident record and fill it with secret-free references:

\`\`\`text
Environment:
Change / incident ID:
Owner / incident lead:
Approver:
ORBIT version:
Commit SHA:
Artifact or image digest:
Configuration revision:
CI run links:
OIDC provider reference (not credentials):
DNS / certificate evidence:
Firewall / network-policy evidence:
Database migration ledger:
Backup ID and timestamp:
Restore drill report:
Measured RPO / RTO:
Load-test report (throughput, p50/p95/p99, saturation, recovery):
Workspace A/B isolation result:
Post-deployment health result:
Outstanding risks / accepted exceptions:
Decision and UTC timestamp:
\`\`\`

## 10. Current known limitations

- This runbook is a procedure baseline, not evidence that a target environment exists or that any listed gate has passed.
- The local telemetry retention/cardinality profile is single-tenant and does not provide managed per-workspace quotas.
- The current SLO calculation is based on instrumented HTTP request success and correctly returns \`insufficient_data\` when no workspace-attributed observations exist; do not interpret missing data as 100% availability.
- The current alert evaluator's first policy is limited to the allow-listed \`request_rate\` metric. Do not claim general latency/error-rate alerting without separately sourced metrics and policy support.
- Production tenant ingestion remains disabled until the M06 roadmap's environment-specific gates are completed and evidenced.
