CREATE TABLE IF NOT EXISTS segment_users (
    segment_id UUID NOT NULL REFERENCES segments(id) ON DELETE CASCADE,
    user_id UUID NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    entered_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    PRIMARY KEY (segment_id, user_id)
);

CREATE INDEX IF NOT EXISTS idx_segment_users_segment ON segment_users(segment_id);
CREATE INDEX IF NOT EXISTS idx_segment_users_user ON segment_users(user_id);
