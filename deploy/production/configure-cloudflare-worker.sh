#!/usr/bin/env bash
set -Eeuo pipefail
umask 077

ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT/.env.production"
TEMP_DIR=""

cleanup() {
  if [[ -n "$TEMP_DIR" && -d "$TEMP_DIR" ]]; then
    rm -rf "$TEMP_DIR"
  fi
}
trap cleanup EXIT

[[ -f "$ENV_FILE" ]] || { echo "Run deploy/production/free-stack.sh prepare first." >&2; exit 1; }
command -v npx >/dev/null 2>&1 || { echo "Node.js/npm (npx) is required to configure the Cloudflare Worker." >&2; exit 1; }

set -a
# shellcheck disable=SC1090
source "$ENV_FILE"
set +a

[[ "${ORBIT_API_DOMAIN:-}" =~ ^[a-z0-9-]+\.duckdns\.org$ ]] || { echo "ORBIT_API_DOMAIN in .env.production is invalid." >&2; exit 1; }
[[ "${ORBIT_WORKER_GATEWAY_SECRET:-}" =~ ^[a-f0-9]{64}$ ]] || { echo "ORBIT_WORKER_GATEWAY_SECRET in .env.production is invalid." >&2; exit 1; }

read -r -p "Confirm Cloudflare Dashboard shows Workers Free (not Workers Paid / Standard) before deployment [y/N]: " FREE_PLAN_CONFIRMED
case "$FREE_PLAN_CONFIRMED" in
  y|Y|yes|YES) ;;
  *) echo "Deployment stopped. Verify the Workers Free plan first; no Worker update was made." >&2; exit 1 ;;
esac

WRANGLER_VERSION="4.158.0"
npx --yes "wrangler@$WRANGLER_VERSION" whoami >/dev/null

TEMP_DIR="$(mktemp -d)"
SECRET_FILE="$TEMP_DIR/worker-secrets.env"
printf 'ORBIT_GATEWAY_SECRET=%s\n' "$ORBIT_WORKER_GATEWAY_SECRET" > "$SECRET_FILE"
chmod 600 "$SECRET_FILE"

npx --yes "wrangler@$WRANGLER_VERSION" deploy \
  --config "$ROOT/cloudflare/wrangler.jsonc" \
  --var "ORBIT_API_ORIGIN:https://$ORBIT_API_DOMAIN" \
  --secrets-file "$SECRET_FILE" \

echo "Cloudflare Worker updated. Secret values were not printed."
