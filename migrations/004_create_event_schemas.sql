CREATE TABLE IF NOT EXISTS event_schemas (
    id UUID PRIMARY KEY DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    event_name VARCHAR(255) NOT NULL,
    property_name VARCHAR(255) NOT NULL,
    property_type VARCHAR(50) NOT NULL DEFAULT 'string',
    distinct_values_count INT NOT NULL DEFAULT 0,
    first_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    last_seen TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    UNIQUE(project_id, event_name, property_name)
);

CREATE INDEX IF NOT EXISTS idx_event_schemas_project_event
    ON event_schemas(project_id, event_name);
