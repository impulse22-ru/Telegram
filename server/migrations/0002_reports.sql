-- Жалобы на видео от пользователей (модерация / фильтрация ленты).
CREATE TABLE IF NOT EXISTS reports (
    id          BIGSERIAL PRIMARY KEY,
    user_id     BIGINT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    video_id    BIGINT NOT NULL REFERENCES videos(id) ON DELETE CASCADE,
    reason      TEXT NOT NULL DEFAULT '',
    status      TEXT NOT NULL DEFAULT 'open',   -- open | reviewed | dismissed
    created_at  TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (user_id, video_id)
);
CREATE INDEX IF NOT EXISTS reports_status_idx ON reports (status, created_at);
CREATE INDEX IF NOT EXISTS reports_video_idx  ON reports (video_id);