#!/bin/sh
set -eu

# Local Compose does not need Keycloak; skip this optional database unless a
# dedicated production password has been supplied.
if [ -z "${KEYCLOAK_DB_PASSWORD:-}" ]; then
  echo "KEYCLOAK_DB_PASSWORD is unset; skipping optional Keycloak database."
  exit 0
fi

psql --username "$POSTGRES_USER" --dbname "$POSTGRES_DB" \
  --set=keycloak_password="$KEYCLOAK_DB_PASSWORD" <<'SQL'
CREATE ROLE orbit_keycloak LOGIN PASSWORD :'keycloak_password';
CREATE DATABASE keycloak OWNER orbit_keycloak;
SQL
