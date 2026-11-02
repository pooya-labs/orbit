# PostgreSQL backup and restore runbook

## Scope and safety

ORBIT stores workspace membership, audit events, incident timelines, alert lifecycle history, notification attempts, deployment annotations, SLO definitions, telemetry catalog metadata, and the schema migration ledger in PostgreSQL. A backup must cover the **entire database**, not a hand-picked subset of tables.

This runbook defines a portable logical snapshot using PostgreSQL's custom archive format. It is a procedure, not a backup scheduler or a managed backup service. The repository does not provision off-host storage, encryption keys, PITR/WAL archiving, retention schedules, or production credentials.

The CI restore drill runs only against its disposable PostgreSQL test database. The script creates one test workspace and one audit event in its source database before taking the dump; do not point `ORBIT_TEST_DATABASE_URL` at production or a database whose contents must remain unchanged.

## Create a backup

Use a `pg_dump` client matching the database major version (or newer):

```sh
pg_dump \
  --format=custom \
  --no-owner \
  --no-acl \
  --file="orbit-$(date -u +%Y%m%dT%H%M%SZ).dump" \
  "$ORBIT_DATABASE_URL"
```

The archive contains schema, application data, workspace/audit history, and `schema_migrations`. `--no-owner --no-acl` makes the logical archive portable between database roles; recreate database roles, grants, and connection credentials separately from the deployment's secret/IaC system.

Immediately encrypt the archive with the approved KMS/age/secret-management workflow before copying it off-host. Restrict access to the backup location: audit rows contain server-side identity issuer/subject values even though those values are not returned to the browser. Never commit a dump, populated environment file, database URL, or encryption key to Git.

A successful `pg_dump` exit code means the logical export completed. It does not prove that the archive can be restored; keep the archive checksum and restore-verification result with the backup metadata.

## Restore into a new database

Restore into a **new, empty database** first. Do not overwrite the live database during verification:

```sh
createdb --template=template0 orbit_restore_verify
pg_restore \
  --exit-on-error \
  --no-owner \
  --no-acl \
  --dbname=orbit_restore_verify \
  orbit-backup.dump
```

Verify the schema ledger and representative durable records before cutover:

```sh
psql -d orbit_restore_verify -Atc \
  "SELECT version FROM schema_migrations ORDER BY version"

psql -d orbit_restore_verify -Atc \
  "SELECT count(*) FROM workspace_audit_events"
```

Also verify application compatibility with the restored schema, required roles/grants, extensions, and connection/TLS settings. Run the API's readiness check and read-only workspace queries against the restored target before any controlled cutover. A rollback plan must identify the original database and preserve the pre-cutover snapshot.

## Repeatable restore drill

The repository script `scripts/postgres_backup_restore_smoke.sh` performs a full custom-format dump/restore round-trip against a disposable database. It:

1. Creates a test workspace, owner membership, and a durable audit event in the source.
2. Dumps the full database and restores it into a separate database created from `template0`.
3. Compares the complete ordered `schema_migrations` ledger.
4. Verifies the workspace ID, audit event, pseudonymous actor reference, and captured role survived the restore.
5. Confirms audit events and actor-reference mappings still reject UPDATE after restoration.
6. Drops only the temporary restored database; the disposable source is left intact apart from the inserted test fixture.

Example environment for a local disposable PostgreSQL instance:

```sh
export ORBIT_TEST_DATABASE_URL="postgres://orbit:orbit_test_only@127.0.0.1:5432/orbit_test?sslmode=disable"
export PGHOST=127.0.0.1
export PGPORT=5432
export PGUSER=orbit
export PGPASSWORD=orbit_test_only
export PGSSLMODE=disable
sh scripts/postgres_backup_restore_smoke.sh
```

The script requires `psql`, `pg_dump`, `pg_restore`, `createdb`, and `dropdb`. CI runs it with the matching `postgres:17-alpine` client image against its ephemeral PostgreSQL 17 service.

## Production go-live requirements

Before production tenant ingestion is enabled, the deployment owner must configure and verify:

- Encrypted off-host backup storage with access logging and least-privilege access.
- A documented snapshot schedule and retention period approved for the deployment's compliance and data-retention requirements.
- PITR/WAL archiving or an explicitly accepted snapshot-only recovery point, according to the target RPO.
- A restore rehearsal against the intended managed PostgreSQL version and topology, including application startup and representative workspace/audit reads.
- Measured and recorded RPO/RTO, alerting for failed/missing backups, key rotation, and an incident owner/runbook.

The CI round-trip is an automated regression gate, **not** evidence that any production backup schedule, encryption, off-site durability, PITR, RPO/RTO, or managed-service restore has been configured. Durable audit, incident, alert, notification, and deployment histories remain append-only; this drill does not introduce TTL deletion or a retention policy for those records.
