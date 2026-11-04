#!/usr/bin/env bash
set -Eeuo pipefail
umask 077
ROOT="$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd)"
ENV_FILE="$ROOT/.env.production"

case "${1:-}" in
  prepare)
    [[ ! -e "$ENV_FILE" ]] || { echo "$ENV_FILE already exists; refusing to overwrite." >&2; exit 1; }
    read -r -p "API hostname (e.g. orbit-api.duckdns.org): " API_DOMAIN
    read -r -p "Auth hostname (e.g. orbit-auth.duckdns.org): " AUTH_DOMAIN
    read -r -p "OTLP/HTTP hostname: " OTLP_HTTP_DOMAIN
    read -r -p "OTLP/gRPC hostname: " OTLP_GRPC_DOMAIN
    read -r -s -p "DuckDNS token: " DUCKDNS_TOKEN
    printf '\n'
    read -r -p "OCI Object Storage bucket name (e.g. orbit-backups): " ORBIT_BACKUP_BUCKET
    read -r -p "OCI Object Storage namespace: " ORBIT_BACKUP_NAMESPACE
    read -r -p "Offline age public recipient (age1...): " ORBIT_BACKUP_AGE_RECIPIENT
    [[ -n "$ORBIT_BACKUP_BUCKET" && -n "$ORBIT_BACKUP_NAMESPACE" ]] || { echo "Bucket and namespace are required." >&2; exit 1; }
    [[ "$ORBIT_BACKUP_AGE_RECIPIENT" =~ ^age1[0-9a-z]{20,}$ ]] || { echo "A valid age public recipient is required." >&2; exit 1; }
    for domain in "$API_DOMAIN" "$AUTH_DOMAIN" "$OTLP_HTTP_DOMAIN" "$OTLP_GRPC_DOMAIN"; do
      [[ "$domain" == *.duckdns.org ]] || { echo "All hostnames must end in .duckdns.org." >&2; exit 1; }
    done
    [[ "$API_DOMAIN" != "$AUTH_DOMAIN" && "$API_DOMAIN" != "$OTLP_HTTP_DOMAIN" && "$API_DOMAIN" != "$OTLP_GRPC_DOMAIN" &&
       "$AUTH_DOMAIN" != "$OTLP_HTTP_DOMAIN" && "$AUTH_DOMAIN" != "$OTLP_GRPC_DOMAIN" && "$OTLP_HTTP_DOMAIN" != "$OTLP_GRPC_DOMAIN" ]] ||
       { echo "Use four distinct DuckDNS names." >&2; exit 1; }
    POSTGRES_PASSWORD="$(openssl rand -hex 32)"
    KEYCLOAK_DB_PASSWORD="$(openssl rand -hex 32)"
    KEYCLOAK_ADMIN_PASSWORD="$(openssl rand -hex 32)"
    DUCKDNS_DOMAINS="${API_DOMAIN%.duckdns.org},${AUTH_DOMAIN%.duckdns.org},${OTLP_HTTP_DOMAIN%.duckdns.org},${OTLP_GRPC_DOMAIN%.duckdns.org}"
    cat > "$ENV_FILE" <<EOF
ORBIT_WEB_ORIGIN=https://pooya-labs.github.io
ORBIT_API_DOMAIN=$API_DOMAIN
ORBIT_AUTH_DOMAIN=$AUTH_DOMAIN
ORBIT_OTLP_HTTP_DOMAIN=$OTLP_HTTP_DOMAIN
ORBIT_OTLP_GRPC_DOMAIN=$OTLP_GRPC_DOMAIN
ORBIT_DATABASE_URL=postgres://orbit:$POSTGRES_PASSWORD@postgres:5432/orbit?sslmode=disable
POSTGRES_PASSWORD=$POSTGRES_PASSWORD
KEYCLOAK_DB_PASSWORD=$KEYCLOAK_DB_PASSWORD
KEYCLOAK_ADMIN_USER=orbit-admin
KEYCLOAK_ADMIN_PASSWORD=$KEYCLOAK_ADMIN_PASSWORD
ORBIT_OIDC_ISSUER=https://$AUTH_DOMAIN/realms/orbit
ORBIT_OIDC_AUDIENCE=orbit-api
ORBIT_WORKLOAD_OIDC_ISSUER=https://$AUTH_DOMAIN/realms/orbit
ORBIT_WORKLOAD_OIDC_AUDIENCE=orbit-ingestion
DUCKDNS_DOMAINS=$DUCKDNS_DOMAINS
DUCKDNS_TOKEN=$DUCKDNS_TOKEN
ORBIT_BACKUP_BUCKET=$ORBIT_BACKUP_BUCKET
ORBIT_BACKUP_NAMESPACE=$ORBIT_BACKUP_NAMESPACE
ORBIT_BACKUP_AGE_RECIPIENT=$ORBIT_BACKUP_AGE_RECIPIENT
ORBIT_BACKUP_MAX_BUCKET_BYTES=16000000000
EOF
    chmod 600 "$ENV_FILE"
    echo "Created $ENV_FILE (mode 600). Keep it private; it contains all bootstrap secrets."
    ;;
  update-dns)
    [[ -f "$ENV_FILE" ]] || { echo "Run free-stack.sh prepare first." >&2; exit 1; }
    set -a; source "$ENV_FILE"; set +a
    response="$(curl --fail --silent --show-error --get \
      --data-urlencode "domains=$DUCKDNS_DOMAINS" \
      --data-urlencode "token=$DUCKDNS_TOKEN" \
      "https://www.duckdns.org/update")"
    [[ "$response" == "OK" ]] || { echo "DuckDNS update failed." >&2; exit 1; }
    ;;
  start)
    [[ -f "$ENV_FILE" ]] || { echo "Run free-stack.sh prepare first." >&2; exit 1; }
    cd "$ROOT"
    COMPOSE=(docker compose --env-file "$ENV_FILE" -f docker-compose.yml -f deploy/production/compose.ingress.yaml --profile observability --profile production-ingress)
    "${COMPOSE[@]}" config --quiet
    "${COMPOSE[@]}" up -d postgres prometheus loki tempo otel-collector keycloak api caddy
    "${COMPOSE[@]}" ps
    ;;
  *)
    echo "Usage: $0 {prepare|update-dns|start}" >&2
    exit 2
    ;;
esac
