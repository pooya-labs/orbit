#!/usr/bin/env sh
# Full PostgreSQL custom-format dump/restore regression drill.
# This never changes or drops the source database; it restores into a new DB.
set -eu

: "${ORBIT_TEST_DATABASE_URL:?ORBIT_TEST_DATABASE_URL must point to the source PostgreSQL database}"
: "${PGHOST:?PGHOST must be set for createdb/dropdb/pg_restore}"
: "${PGPORT:?PGPORT must be set for createdb/dropdb/pg_restore}"
: "${PGUSER:?PGUSER must be set for createdb/dropdb/pg_restore}"
: "${PGPASSWORD:?PGPASSWORD must be set for createdb/dropdb/pg_restore}"

for command in psql pg_dump pg_restore createdb dropdb; do
  command -v "$command" >/dev/null 2>&1 || {
    echo "ERROR: required PostgreSQL client command is missing: $command" >&2
    exit 1
  }
done

work_dir="$(mktemp -d)"
restore_db="orbit_restore_drill_$$"
cleanup() {
  dropdb --if-exists "$restore_db" >/dev/null 2>&1 || true
  rm -rf "$work_dir"
}
trap cleanup EXIT HUP INT TERM

archive="$work_dir/orbit.dump"
fixture_suffix="$(date +%s)_$$"
workspace_name="Backup restore drill $fixture_suffix"
workspace_slug="backup-restore-$fixture_suffix"
issuer="https://identity.example.test/"
subject="backup-restore-$fixture_suffix"

echo "[backup-restore] creating a durable workspace and audit fixture"
workspace_id="$(
  psql "$ORBIT_TEST_DATABASE_URL" -X -qAt -v ON_ERROR_STOP=1 \
    -v workspace_name="$workspace_name" \
    -v workspace_slug="$workspace_slug" \
    -v issuer="$issuer" \
    -v subject="$subject" <<'SQL'
BEGIN;
INSERT INTO workspaces (name, slug, created_by_issuer, created_by_subject)
VALUES (:'workspace_name', :'workspace_slug', :'issuer', :'subject')
RETURNING id::text AS fixture_workspace_id \gset

INSERT INTO workspace_memberships (workspace_id, user_issuer, user_subject, role)
VALUES (:'fixture_workspace_id'::uuid, :'issuer', :'subject', 'owner');

INSERT INTO workspace_audit_events (
  workspace_id, actor_issuer, actor_subject, action, resource_type, resource_id, details
)
VALUES (
  :'fixture_workspace_id'::uuid, :'issuer', :'subject',
  'backup_restore.drill', 'workspace', :'fixture_workspace_id',
  '{"purpose":"postgres-backup-restore-drill"}'::jsonb
);
COMMIT;
SELECT :'fixture_workspace_id';
SQL
)"

if [ -z "$workspace_id" ]; then
  echo "ERROR: failed to create the source fixture workspace" >&2
  exit 1
fi

source_migrations="$(
  psql "$ORBIT_TEST_DATABASE_URL" -X -qAt -v ON_ERROR_STOP=1 \
    -c "SELECT string_agg(version, ',' ORDER BY version) FROM schema_migrations"
)"
case ",$source_migrations," in
  *,015_workspace_audit_actor_context,*) ;;
  *)
    echo "ERROR: source database is missing migration 015_workspace_audit_actor_context" >&2
    exit 1
    ;;
esac

source_event_count="$(
  psql "$ORBIT_TEST_DATABASE_URL" -X -qAt -v ON_ERROR_STOP=1 \
    -v workspace_id="$workspace_id" \
    -c "SELECT COUNT(*) FROM workspace_audit_events WHERE workspace_id = :'workspace_id'::uuid AND action = 'backup_restore.drill'"
)"
[ "$source_event_count" = "1" ] || {
  echo "ERROR: source audit fixture count is $source_event_count, expected 1" >&2
  exit 1
}

echo "[backup-restore] creating a custom-format full database dump"
pg_dump --format=custom --no-owner --no-acl --file="$archive" "$ORBIT_TEST_DATABASE_URL"
test -s "$archive"
pg_restore --list "$archive" >/dev/null

echo "[backup-restore] restoring into isolated database $restore_db"
createdb --template=template0 "$restore_db"
pg_restore --exit-on-error --no-owner --no-acl --dbname="$restore_db" "$archive"

restored_migrations="$(
  psql -X -qAt -v ON_ERROR_STOP=1 -d "$restore_db" \
    -c "SELECT string_agg(version, ',' ORDER BY version) FROM schema_migrations"
)"
[ "$source_migrations" = "$restored_migrations" ] || {
  echo "ERROR: schema migration ledger differs after restore" >&2
  echo "source:   $source_migrations" >&2
  echo "restored: $restored_migrations" >&2
  exit 1
}

restored_workspace_id="$(
  psql -X -qAt -v ON_ERROR_STOP=1 -d "$restore_db" \
    -v workspace_slug="$workspace_slug" \
    -c "SELECT id::text FROM workspaces WHERE slug = :'workspace_slug'"
)"
[ "$workspace_id" = "$restored_workspace_id" ] || {
  echo "ERROR: restored workspace identity does not match the source fixture" >&2
  exit 1
}

restored_actor_context="$(
  psql -X -qAt -v ON_ERROR_STOP=1 -d "$restore_db" \
    -v workspace_id="$workspace_id" \
    -c "SELECT actor_ref::text || '|' || actor_role FROM workspace_audit_events WHERE workspace_id = :'workspace_id'::uuid AND action = 'backup_restore.drill'"
)"
case "$restored_actor_context" in
  ?*'|owner') ;;
  *)
    echo "ERROR: restored audit actor context is missing or incorrect: $restored_actor_context" >&2
    exit 1
    ;;
esac

restored_actor_ref="${restored_actor_context%|owner}"
actor_mapping_count="$(
  psql -X -qAt -v ON_ERROR_STOP=1 -d "$restore_db" \
    -v workspace_id="$workspace_id" -v actor_ref="$restored_actor_ref" \
    -c "SELECT COUNT(*) FROM workspace_audit_actors WHERE workspace_id = :'workspace_id'::uuid AND actor_ref = :'actor_ref'::uuid"
)"
[ "$actor_mapping_count" = "1" ] || {
  echo "ERROR: pseudonymous actor mapping was not restored" >&2
  exit 1
}

if psql -X -q -v ON_ERROR_STOP=1 -d "$restore_db" \
  -v workspace_id="$workspace_id" \
  -c "UPDATE workspace_audit_events SET action = 'tampered' WHERE workspace_id = :'workspace_id'::uuid" >/dev/null 2>&1; then
  echo "ERROR: restored audit events are mutable" >&2
  exit 1
fi

if psql -X -q -v ON_ERROR_STOP=1 -d "$restore_db" \
  -v workspace_id="$workspace_id" \
  -c "UPDATE workspace_audit_actors SET actor_ref = gen_random_uuid() WHERE workspace_id = :'workspace_id'::uuid" >/dev/null 2>&1; then
  echo "ERROR: restored audit actor mappings are mutable" >&2
  exit 1
fi

echo "[backup-restore] PASS: full dump/restore preserved migration ledger, workspace data, actor context, and append-only guards"
