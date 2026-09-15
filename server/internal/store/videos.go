package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type videosRepo struct{ pg *pgxpool.Pool }

func (r *videosRepo) Insert(ctx context.Context, v Video) error {
	tags := v.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := r.pg.Exec(ctx, `
		INSERT INTO videos (tg_msg_id, file_id, caption, duration_ms, width, height, title, tags, channel_id, posted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10)
		ON CONFLICT (channel_id, tg_msg_id) DO NOTHING`,
		v.TgMsgID, v.FileID, v.Caption, v.DurationMs, v.Width, v.Height, v.Title, tags, v.ChannelID, v.PostedAt)
	return err
}

func (r *videosRepo) VisibleFrom(ctx context.Context, channelID, after, limit int64) ([]Video, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(caption,''), COALESCE(duration_ms,0),
		       COALESCE(width,0), COALESCE(height,0), COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos
		WHERE channel_id = $1 AND status = 'visible' AND id > $2
		ORDER BY id
		LIMIT $3`, channelID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Video, 0, limit)
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.Caption, &v.DurationMs,
			&v.Width, &v.Height, &v.Title, &v.Tags, &v.Status, &v.ChannelID, &v.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *videosRepo) Ban(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='banned' WHERE id=$1`, id)
	return err
}

func (r *videosRepo) Unban(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='visible' WHERE id=$1`, id)
	return err
}

func (r *videosRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='deleted' WHERE id=$1`, id)
	return err
}

// Search — поиск по title/caption (для команды !search в боте).
func (r *videosRepo) Search(ctx context.Context, q string, limit int64) ([]Video, error) {
	if limit <= 0 {
		limit = 10
	}
	pattern := "%" + q + "%"
	rows, err := r.pg.Query(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(caption,''), COALESCE(duration_ms,0),
		       COALESCE(width,0), COALESCE(height,0), COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos
		WHERE status='visible' AND (title ILIKE $1 OR COALESCE(caption,'') ILIKE $1)
		ORDER BY posted_at DESC
		LIMIT $2`, pattern, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanVideos(rows)
}

// Count — число видео по статусу ('' = всего).
func (r *videosRepo) Count(ctx context.Context, status string) (int64, error) {
	var n int64
	var err error
	if status == "" {
		err = r.pg.QueryRow(ctx, `SELECT count(*) FROM videos`).Scan(&n)
	} else {
		err = r.pg.QueryRow(ctx, `SELECT count(*) FROM videos WHERE status=$1`, status).Scan(&n)
	}
	return n, err
}

func (r *videosRepo) Get(ctx context.Context, id int64) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(caption,''), COALESCE(duration_ms,0),
		       COALESCE(width,0), COALESCE(height,0), COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos WHERE id=$1`, id).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.Caption, &v.DurationMs,
			&v.Width, &v.Height, &v.Title, &v.Tags, &v.Status, &v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func (r *videosRepo) GetByTgMsg(ctx context.Context, channelID, tgMsgID int64) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(caption,''), COALESCE(duration_ms,0),
		       COALESCE(width,0), COALESCE(height,0), COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos WHERE channel_id=$1 AND tg_msg_id=$2`, channelID, tgMsgID).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.Caption, &v.DurationMs,
			&v.Width, &v.Height, &v.Title, &v.Tags, &v.Status, &v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

func scanVideos(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]Video, error) {
	out := make([]Video, 0)
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.Caption, &v.DurationMs,
			&v.Width, &v.Height, &v.Title, &v.Tags, &v.Status, &v.ChannelID, &v.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ Videos = (*videosRepo)(nil)