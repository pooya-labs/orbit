# ORBIT — zero-cost infrastructure deployment

**Hard budget: $0/month.** Do not add paid plans, usage-billed services,
managed databases, paid domains, paid Cloudflare add-ons or automatic paid
overages. If an Always Free resource is unavailable, wait or use hardware you
already own; never choose a paid fallback.

## Architecture

| Component | Implementation |
|---|---|
| Dashboard | Existing GitHub Pages static export |
| API | Existing Go API container on one OCI Always Free VM |
| Database | Self-hosted PostgreSQL container and persistent volume |
| Identity | Self-hosted Keycloak 26.7.3 |
| Telemetry | Existing Collector, Prometheus, Loki and Tempo containers |
| HTTPS ingress | Caddy with automatic Let's Encrypt certificates |
| DNS | Four free DuckDNS hostnames |
| Backup target | Oracle Object Storage Always Free, only within its free quota |

This avoids Railway, Render, Neon, Grafana Cloud and managed PostgreSQL. The
deployment is a single-node design with no HA failover.

## Free resource limits

Oracle's current Always Free allowance for Ampere A1 is equivalent to one VM
with **2 OCPUs and 12 GB RAM**; Always Free includes **200 GB total Block
Volume**, up to **five volume backups**, and **20 GB combined Object Storage**
for an Always-Free-only tenancy. These limits and home-region eligibility are
listed in the [official OCI Always Free resource page](https://docs.oracle.com/en-us/iaas/Content/FreeTier/freetier_topic-Always_Free_Resources.htm).
A1 capacity may be temporarily unavailable. Do not upgrade to Pay As You Go or
select a paid shape to bypass that.

Create an ARM64 Ubuntu 24.04 VM with 2 OCPUs, 12 GB RAM and 100 GB boot volume.
Confirm every selected item is labelled **Always Free eligible**. Restrict SSH
TCP 22 to your IP; expose only TCP 80/443 (and optionally UDP 443). Never open
API 8080, ingestion 4319/4320, Collector 4317/4318, PostgreSQL 5432, Prometheus
9090, Loki 3100, Tempo 3200 or Keycloak management 9000.

## DNS and secrets

Create four distinct subdomains in [DuckDNS](https://www.duckdns.org/), for
example:

- orbit-api.duckdns.org
- orbit-auth.duckdns.org
- orbit-otlp-http.duckdns.org
- orbit-otlp-grpc.duckdns.org

DuckDNS supports updating several names with a single token-based API call;
see its [official update specification](https://www.duckdns.org/spec.jsp).

Clone the repository on the VM, then run:

```sh
./deploy/production/free-stack.sh prepare
```

Enter the four full DuckDNS names and token. The script generates independent
random PostgreSQL and Keycloak passwords and writes a git-ignored
`.env.production` file with permissions 600. Never commit or share this file.

Update DNS now and schedule it every five minutes using root's crontab:

```sh
./deploy/production/free-stack.sh update-dns
sudo crontab -e
```

Add a cron entry using the actual repository path:

```cron
*/5 * * * * /absolute/path/orbit/deploy/production/free-stack.sh update-dns >/dev/null 2>&1
```

Caddy obtains HTTPS certificates after DNS resolves to the VM and TCP 80/443
are reachable.

## Start the production stack

Install Docker Engine and the Compose plugin on the VM, then:

```sh
./deploy/production/free-stack.sh start
```

PostgreSQL creates a separate `keycloak` database on first volume
initialization. This requires a fresh production database volume. If the volume
was initialized earlier, create the `orbit_keycloak` role and `keycloak`
database manually before starting Keycloak.

Only Caddy publishes public ports. Caddy can reach only the API and Keycloak;
it is not connected to PostgreSQL or the private telemetry network. API,
database, Collector and backend listeners stay unpublished.

## Configure Keycloak

Open `https://<auth-host>/` and use the bootstrap admin credentials in
`.env.production`.

1. Create a realm named `orbit`.
2. Create realm roles exactly `viewer`, `operator` and `administrator`.
3. Create public SPA client `orbit-web`: client authentication Off, standard
   flow On, direct access grants Off, PKCE S256 required. Set redirect URI to
   `https://pooya-labs.github.io/orbit/auth/callback/` and Web Origin to
   `https://pooya-labs.github.io`.
4. Add an Audience mapper to the access token with custom audience
   `orbit-api`. Add a User Realm Role mapper to the access token with claim
   name `roles`, multivalued enabled, and no role prefix.
5. Create a separate confidential client with Service Accounts enabled for
   each workload. Use client-credentials issuance and only the minimum
   service-account grants required by that workload.
6. On each workload client enable **Use at+jwt as access token header type**
   in Advanced / Fine Grain OpenID Connect Configuration. Keycloak supports
   this per-client option from 26.2 onward; see the
   [official release notes](https://www.keycloak.org/docs/latest/release_notes/index).
   Add an Audience mapper for `orbit-ingestion` and a hardcoded numeric
   `nbf=0` claim to the access token because the current ORBIT workload verifier
   requires that claim to be present. ORBIT still verifies issuer, signature,
   key ID, audience, algorithm, expiration and issued-at.
7. Set the issuer exactly to
   `https://<auth-host>/realms/orbit`; use distinct audiences
   `orbit-api` and `orbit-ingestion`.

## Connect GitHub Pages

In repository Settings → Secrets and variables → Actions → Variables, set these
public build-time values (they are not secrets):

- `NEXT_PUBLIC_ORBIT_API_URL=https://<api-host>`
- `NEXT_PUBLIC_ORBIT_OIDC_ISSUER=https://<auth-host>/realms/orbit`
- `NEXT_PUBLIC_ORBIT_OIDC_CLIENT_ID=orbit-web`
- `NEXT_PUBLIC_ORBIT_OIDC_AUDIENCE=orbit-api`

The Pages workflow reads these values during the static build. Rerun the
**Deploy ORBIT to GitHub Pages** workflow after saving them. API CORS is
restricted to the exact Pages origin.

## Persistence and limits

- Do not run `docker compose down -v` on the production VM.
- PostgreSQL, Prometheus, Loki and Tempo use persistent named volumes.
- The local telemetry retention defaults are seven days. Durable workspace
  audit, incident, alert, notification and deployment histories are not
  automatically deleted.
- OCI Always Free Object Storage and volume backups are available within their
  quotas, but this repository does not yet schedule encrypted backups. Before
  accepting real tenant ingestion, create an Always Free bucket, grant the
  instance principal access, configure encrypted quota-bounded PostgreSQL
  backups and complete one restore.
- One VM is a single point of failure. This configuration is for the initial
  small-scale release and does not claim HA or certified capacity.

## Before and after v1

Before v1, perform only minimal deployment smoke checks: API liveness/readiness,
version metadata, real browser login, API role authorization, one controlled
metrics/logs/traces write-query, and A/B workspace isolation. Keep tenant
ingestion disabled until those checks pass.

Human TalkBack/VoiceOver review, broad accessibility acceptance, sustained
load/capacity benchmarking and longer operational rehearsals are deferred
until after v1. Do not claim WCAG conformance or production-scale capacity
before those follow-ups.
