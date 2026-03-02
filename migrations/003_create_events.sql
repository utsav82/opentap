CREATE TABLE IF NOT EXISTS events (
    id UUID DEFAULT uuid_generate_v4(),
    project_id UUID NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    event_name VARCHAR(255) NOT NULL,
    properties JSONB DEFAULT '{}',
    timestamp TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    received_at TIMESTAMPTZ NOT NULL DEFAULT NOW()
);

SELECT create_hypertable('events', 'timestamp', if_not_exists => TRUE);

CREATE INDEX IF NOT EXISTS idx_events_project_name_time
    ON events (project_id, event_name, timestamp DESC);

CREATE INDEX IF NOT EXISTS idx_events_project_user_time
    ON events (project_id, user_id, timestamp DESC);

-- Retention policy
SELECT add_retention_policy('events', INTERVAL '90 days', if_not_exists => TRUE);
