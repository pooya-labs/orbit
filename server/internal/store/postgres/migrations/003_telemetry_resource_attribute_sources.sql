ALTER TABLE telemetry_services
    ADD COLUMN resource_attribute_sources JSONB NOT NULL DEFAULT '{}'::jsonb;

CREATE INDEX telemetry_services_resource_attributes_gin_idx
    ON telemetry_services USING GIN (resource_attributes);
