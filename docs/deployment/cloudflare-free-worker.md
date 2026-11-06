# Deploy ORBIT to Cloudflare Workers Free

This runbook deploys the repository's Cloudflare edge gateway on the Workers Free plan. It does not replace the existing Go/PostgreSQL/Keycloak/Collector origin and does not change the Apps Script demo.

## 0. Billing gate — confirm the Free plan

1. Sign in to the Cloudflare Dashboard and select the ORBIT account.
2. Open **Manage Account → Billing → Subscriptions**.
3. Confirm the Workers row says **Workers Free — Active** and that no Workers Paid subscription is active. The owner-provided dashboard view already showed exactly this state, with **Upgrade to Workers Paid** and **No payment method on file**.
4. Do not upgrade, add a payment method, enable automatic overages, or change a subscription for this project.
5. The Worker settings API currently reports `usage_model: standard`, while the Billing page confirms Workers Free is Active. Treat the Billing subscription page as the account-plan source of truth; leave the API usage-model field unchanged and do not set the legacy `bundled` value.

Cloudflare API access can read Worker configuration and versions but the connected OAuth integration receives HTTP 403 for `/accounts/{account_id}/subscriptions` because that endpoint requires Billing Read/Write API-token permission. No billing or subscription change was made.

Official references: [Workers pricing and Free limits](https://developers.cloudflare.com/workers/platform/pricing/) and [Workers limits](https://developers.cloudflare.com/workers/platform/limits/).

## 1. Get the repository and validate the code

Use the branch containing the Cloudflare work after its PR checks are green. From a terminal:

```bash
git clone https://github.com/pooya-labs/orbit.git
cd orbit
git fetch origin pull/136/head:feat/cloudflare-dual-runtime
git checkout feat/cloudflare-dual-runtime
npm ci
npm run test:cloudflare-gateway
secret_file="$(mktemp)"
trap 'rm -f "$secret_file"' EXIT
printf 'ORBIT_GATEWAY_SECRET=%064d\n' 0 > "$secret_file"
npx --yes wrangler@4.158.0 deploy \
  --config cloudflare/wrangler.jsonc \
  --var ORBIT_API_ORIGIN:https://api.example.test \
  --secrets-file "$secret_file" \
  --dry-run
```

Workers Free enforces its 10 ms CPU ceiling automatically; this project does not set `limits.cpu_ms`, because custom Wrangler `limits` are documented for the Standard usage model. The Worker has no D1/KV/R2/Queue/DO bindings. The dry run validates packaging only; it does not prove that the API origin is ready.

## 2. Prepare the original ORBIT origin

The Cloudflare Worker is only an edge proxy. It needs the existing Go API behind Caddy, backed by PostgreSQL and Keycloak, with the existing private Collector/Prometheus/Loki/Tempo pipeline. Complete [the original free-only origin runbook](free-only-orbit.md) first.

On the machine where the production repository and Docker Compose stack will run:

```bash
bash deploy/production/free-stack.sh prepare
bash deploy/production/free-stack.sh update-dns
bash deploy/production/free-stack.sh start
```

`prepare` creates `.env.production` with mode `600`, including the random 64-character `ORBIT_WORKER_GATEWAY_SECRET`. Keep this file private and out of Git. Complete the runbook's Keycloak, TLS, backup and origin health checks before exposing real API traffic.

## 3. Deploy the Worker code and its private settings

Current Cloudflare inspection (November 6, 2026): account subdomain `pooyagame141` exists; the `orbit-runtime` workers.dev route is currently disabled; no Worker secret is attached yet. The next deployment uses `workers_dev: true`, so it will turn on the public route. Keep API routes fail-closed until the origin hostname and gateway secret are supplied by the deployment script.

Run this from the same repository checkout on a machine that has access to `.env.production` and is authenticated to the correct Cloudflare account with Wrangler:

```bash
bash deploy/production/configure-cloudflare-worker.sh
```

The script pins Wrangler 4.158.0, asks for explicit confirmation that Workers Free is active, reads the API hostname and gateway secret from `.env.production`, and deploys `cloudflare/wrangler.jsonc`. Its `workers_dev: true` setting enables the Worker's `workers.dev` route during deployment. It passes the secret through a temporary mode-600 file and deletes that file on exit. It does not print the secret.

Do not paste `ORBIT_WORKER_GATEWAY_SECRET` into the dashboard's plain-text variables, GitHub Actions variables, GitHub Pages, browser code, URLs, or chat. The deployment script installs it as a Worker secret.

If Wrangler is not authenticated, run `npx --yes wrangler@4.158.0 login` in the terminal and select the same Cloudflare account whose account ID is configured in `cloudflare/wrangler.jsonc`. Never paste an API token into this repository.

## 4. Verify the edge and origin boundary

After deployment:

```bash
curl -i https://orbit-runtime.pooyagame141.workers.dev/health/live
curl -i https://orbit-runtime.pooyagame141.workers.dev/health/ready
```

`/health/live` should return HTTP 200 directly from the Worker. `/health/ready` should reach the protected origin and return ready only when the Go API is healthy.

From a machine outside the origin, call the origin API route directly without the gateway secret; Caddy must return HTTP 403. Do not send or display the secret while doing this check. Then use the browser dashboard to sign in through the existing Keycloak PKCE flow and test the Go API through the Worker.

## 5. Point GitHub Pages at the Worker

In GitHub, open **pooya-labs/orbit → Settings → Secrets and variables → Actions → Variables**. Set the public build variables used by `.github/workflows/deploy.yml`:

| Variable | Value |
| --- | --- |
| `NEXT_PUBLIC_ORBIT_API_URL` | `https://orbit-runtime.pooyagame141.workers.dev` |
| `NEXT_PUBLIC_ORBIT_OIDC_ISSUER` | The same Keycloak issuer used by the existing origin |
| `NEXT_PUBLIC_ORBIT_OIDC_CLIENT_ID` | The existing public browser client ID |
| `NEXT_PUBLIC_ORBIT_OIDC_AUDIENCE` | The existing Go API audience (`orbit-api`) |

These are public browser configuration values. Never add the Worker gateway secret to GitHub variables or Pages. Run the existing **Deploy Pages** workflow after setting the variables.

## 6. Acceptance checks

- [ ] Cloudflare Billing → Subscriptions confirms **Workers Free — Active** and no Workers Paid subscription is active. Use this account-level subscription page as the plan source of truth.
- [ ] Do not infer the subscription tier from the per-Worker API `usage_model` field. Cloudflare's API schema defaults that field to `standard`; leave it unchanged and do not force the legacy `bundled` value.
- [ ] The Worker has no unexpected bindings and no custom `limits.cpu_ms` override. Workers Free enforces its 10 ms CPU limit automatically.
- [ ] `/health/live` returns 200 and `/health/ready` reflects origin readiness.
- [ ] Direct Caddy API access without the gateway secret returns 403.
- [ ] Browser login uses the existing Keycloak PKCE client and the browser sends API traffic only to the Worker URL.
- [ ] Viewer/operator/administrator permissions and cross-workspace A/B isolation pass through the Worker.
- [ ] OTLP/HTTP and OTLP/gRPC still use their original dedicated ingress hostnames and authenticated Go gateways.
- [ ] Apps Script synthetic mode still works independently and its deployment/spreadsheet were not changed.

Workers Free currently allows 100,000 requests/day and 10 ms CPU per invocation. Beginning December 1, 2026, Workers Logs and Workers Traces share a Free allowance of 0.5 GB ingestion/day with seven-day retention; Free stops new ingestion when the daily allowance is reached. The Worker samples logs and traces at 10%. If a Free limit is reached, accept the Free-plan limit behavior; do not enable paid overages.

## Stop conditions

Do not deploy or route real tenant traffic unless Cloudflare Billing confirms Workers Free is Active and no Workers Paid subscription is active. The per-Worker `usage_model: standard` metadata value alone is not a stop condition. Also stop if `.env.production` is missing, the Go/Keycloak origin is unhealthy, Caddy does not reject direct API bypass, or any auth/workspace-isolation check fails. Keep the Apps Script demo unchanged.