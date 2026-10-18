CREATE UNIQUE INDEX IF NOT EXISTS idx_events_project_id_time ON events (project_id, id, timestamp);

ALTER TABLE segments ADD COLUMN IF NOT EXISTS last_computed_at TIMESTAMPTZ;
