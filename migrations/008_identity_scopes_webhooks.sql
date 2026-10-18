CREATE TABLE IF NOT EXISTS user_aliases (
    project_id UUID NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
    alias VARCHAR(255) NOT NULL,
    user_id VARCHAR(255) NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (project_id, alias)
);

CREATE INDEX IF NOT EXISTS idx_user_aliases_user ON user_aliases(project_id, user_id);

ALTER TABLE api_keys ADD COLUMN IF NOT EXISTS scope VARCHAR(16) NOT NULL DEFAULT 'full';

ALTER TABLE segments ADD COLUMN IF NOT EXISTS webhook_url TEXT NOT NULL DEFAULT '';
