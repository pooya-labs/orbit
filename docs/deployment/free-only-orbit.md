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
| Identity | Self-hosted Keycloak 26.8.1 |
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
select a paid shape to bypass that. Oracle may reclaim an Always Free VM when
CPU p95, network utilization, and (for A1) memory utilization all remain below
20% over a seven-day period. A low-traffic ORBIT deployment can therefore be
at reclamation risk; Oracle documents this condition explicitly. Do not fake
traffic or use a paid service to work around it.

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

Create the private Standard-tier bucket `orbit-backups` in the VM's home
region, and note the Object Storage namespace shown in the OCI Console. Keep
this bucket dedicated to ORBIT backups. For a zero-cost-only account, the
combined Object Storage allowance is 20 GB and 50,000 requests/month; do not
put unrelated files or other application backups into this quota.

Generate an age key pair on a separate trusted computer (not on the VM):

```sh
age-keygen -o orbit-backup-identity.txt
```

Keep `orbit-backup-identity.txt` offline in a secure place. Only copy the
public `age1...` recipient printed by age-keygen into the VM setup prompt.

Clone the repository at `/opt/orbit`, then run:

```sh
cd /opt/orbit
./deploy/production/free-stack.sh prepare
```

Enter the four full DuckDNS names and token, the bucket name and namespace,
and the age public recipient. The script generates independent random
PostgreSQL and Keycloak passwords and writes a git-ignored
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

Install Docker Engine, the Compose plugin, `age`, `gzip`, `jq`, and OCI CLI
on the VM. Use OCI CLI instance-principal authentication; do not copy an API
signing key onto the server. The official install procedure is documented
[here](https://docs.oracle.com/en-us/iaas/Content/API/SDKDocs/cliinstall.htm).
The Object Storage namespace can be retrieved with
`oci os ns get --auth instance_principal`.

Create a Dynamic Group matching only this VM's OCID, for example:

```text
instance.id = '<ORBIT_VM_OCID>'
```

Then add the following policy in the bucket's compartment (replace the
compartment name if the bucket is not in the root compartment):

```text
Allow dynamic-group orbit-backup-instance to read buckets in compartment <COMPARTMENT_NAME> where target.bucket.name='orbit-backups'
Allow dynamic-group orbit-backup-instance to manage objects in compartment <COMPARTMENT_NAME> where all {target.bucket.name='orbit-backups', any {request.permission='OBJECT_CREATE', request.permission='OBJECT_INSPECT', request.permission='OBJECT_READ'}}
```

The policy permits listing, upload and restore-read for this one bucket but
does not grant object deletion. Oracle documents instance principals and
bucket-scoped Object Storage policies in its
[instance-principal guide](https://docs.oracle.com/en-us/iaas/Content/Identity/Tasks/callingservicesfrominstances.htm)
and [Object Storage policy examples](https://docs.oracle.com/en-us/iaas/Content/Identity/Concepts/commonpolicies.htm).

After the VM's dynamic group and bucket-scoped policy are active, start the stack:

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
- `deploy/production/backup-to-oci.sh` creates an encrypted full PostgreSQL
  cluster dump (ORBIT and Keycloak databases plus roles) using age and uploads
  it with OCI instance-principal authentication. The script checks the
  dedicated bucket's total bytes before every upload and stops at 16 GB,
  leaving 4 GB of headroom below the 20 GB free allocation. It never deletes
  older backups or silently exceeds the cap. Install the provided systemd
  service and daily timer after creating the private bucket and IAM policy.
- The quota check covers the dedicated ORBIT bucket only. Reserve the entire
  tenancy Object Storage allowance for this bucket; the 20 GB Oracle limit is
  combined across tiers/buckets. If other objects already consume that quota,
  reduce the script cap before enabling backups.
- Before accepting real tenant ingestion, complete one isolated restore using
  the offline age identity. Keep that private identity away from the VM.
- One VM is a single point of failure. This configuration is for the initial
  small-scale release and does not claim HA or certified capacity.

## Encrypted backup setup

Install the repository unit files after confirming `/opt/orbit/.env.production`
exists and the VM dynamic group policy is active:

```sh
sudo install -m 0644 deploy/production/orbit-backup.service /etc/systemd/system/
sudo install -m 0644 deploy/production/orbit-backup.timer /etc/systemd/system/
sudo systemctl daemon-reload
sudo systemctl enable --now orbit-backup.timer
sudo systemctl start orbit-backup.service
sudo systemctl status orbit-backup.service --no-pager
```

The daily timer runs at 03:17 UTC with a randomized delay. Logs go to the
system journal. The backup script uses an exclusive lock, dumps every
PostgreSQL database and role, compresses and age-encrypts the stream, checks
the current bucket byte total, uploads without overwriting, and verifies the
uploaded object exists. If OCI auth, dump, encryption, upload or quota checks
fail, the service exits non-zero and retains prior backups.

To restore, download a named object with
`oci os object get --bucket-name "$ORBIT_BACKUP_BUCKET" --namespace "$ORBIT_BACKUP_NAMESPACE" --name "<OBJECT_NAME>" --file backup.sql.gz.age --auth instance_principal`.
On an isolated host, decrypt with the offline identity
(`age -d -i orbit-backup-identity.txt -o backup.sql.gz backup.sql.gz.age`),
then restore the SQL stream into a **new, empty PostgreSQL instance**, never
the live database. Keep the original identity offline; do not place it on the
production VM.

## Before and after v1

Before v1, perform only minimal deployment smoke checks: API liveness/readiness,
version metadata, real browser login, API role authorization, one controlled
metrics/logs/traces write-query, and A/B workspace isolation. Keep tenant
ingestion disabled until those checks pass.

Human TalkBack/VoiceOver review, broad accessibility acceptance, sustained
load/capacity benchmarking and longer operational rehearsals are deferred
until after v1. Do not claim WCAG conformance or production-scale capacity
before those follow-ups.
