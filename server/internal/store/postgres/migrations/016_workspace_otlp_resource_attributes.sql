-- Make the observed telemetry catalog genuinely workspace-scoped.
-- A service name is unique within a workspace, not across all tenants.
ALTER TABLE telemetry_services
    DROP CONSTRAINT IF EXISTS telemetry_services_name_key;

CREATE UNIQUE INDEX IF NOT EXISTS telemetry_services_workspace_name_key
    ON telemetry_services (workspace_id, name);

ALTER TABLE telemetry_service_sources
    DROP CONSTRAINT IF EXISTS telemetry_service_sources_source_check;

ALTER TABLE telemetry_service_sources
    ADD CONSTRAINT telemetry_service_sources_source_check
    CHECK (source IN ('prometheus', 'loki', 'tempo', 'otlp'));
