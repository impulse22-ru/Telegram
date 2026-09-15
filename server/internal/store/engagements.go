package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type engagementsRepo struct{ pg *pgxpool.Pool }

func (r *engagementsRepo) Like(ctx context.Context, userID, videoID int64) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO likes (user_id, video_id) VALUES ($1,$2)
		ON CONFLICT (user_id, video_id) DO NOTHING`, userID, videoID)
	return err
}

func (r *engagementsRepo) Unlike(ctx context.Context, userID, videoID int64) error {
	_, err := r.pg.Exec(ctx, `DELETE FROM likes WHERE user_id=$1 AND video_id=$2`, userID, videoID)
	return err
}

func (r *engagementsRepo) IsLiked(ctx context.Context, userID, videoID int64) (bool, error) {
	var exists bool
	err := r.pg.QueryRow(ctx, `
		SELECT EXISTS(SELECT 1 FROM likes WHERE user_id=$1 AND video_id=$2)`, userID, videoID).Scan(&exists)
	return exists, err
}

func (r *engagementsRepo) CountLikes(ctx context.Context, videoID int64) (int64, error) {
	var n int64
	err := r.pg.QueryRow(ctx, `SELECT count(*) FROM likes WHERE video_id=$1`, videoID).Scan(&n)
	return n, err
}

func (r *engagementsRepo) Comment(ctx context.Context, userID, videoID int64, text string, parentID int64) (int64, error) {
	var id int64
	var parent any
	if parentID > 0 {
		parent = parentID
	}
	err := r.pg.QueryRow(ctx, `
		INSERT INTO comments (user_id, video_id, text, parent_id)
		VALUES ($1,$2,$3,$4) RETURNING id`, userID, videoID, text, parent).Scan(&id)
	return id, err
}

func (r *engagementsRepo) DeleteComment(ctx context.Context, commentID int64) error {
	_, err := r.pg.Exec(ctx, `DELETE FROM comments WHERE id=$1`, commentID)
	return err
}

func (r *engagementsRepo) Comments(ctx context.Context, videoID, limit int64) ([]Comment, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, video_id, text, COALESCE(parent_id,0), created_at
		FROM comments WHERE video_id=$1 ORDER BY id LIMIT $2`, videoID, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Comment, 0, limit)
	for rows.Next() {
		var c Comment
		if err := rows.Scan(&c.ID, &c.UserID, &c.VideoID, &c.Text, &c.ParentID, &c.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, c)
	}
	return out, rows.Err()
}

func (r *engagementsRepo) Report(ctx context.Context, userID, videoID int64, reason string) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO reports (user_id, video_id, reason)
		VALUES ($1,$2,$3)
		ON CONFLICT (user_id, video_id) DO UPDATE SET reason=EXCLUDED.reason, status='open'`,
		userID, videoID, reason)
	return err
}

// Reports — список жалоб со статусом ('' = все).
func (r *engagementsRepo) Reports(ctx context.Context, status string, limit int64) ([]Report, error) {
	if limit <= 0 {
		limit = 50
	}
	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, video_id, COALESCE(reason,''), status, created_at
		FROM reports
		WHERE ($1 = '' OR status = $1)
		ORDER BY created_at DESC
		LIMIT $2`, status, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]Report, 0, limit)
	for rows.Next() {
		var rep Report
		if err := rows.Scan(&rep.ID, &rep.UserID, &rep.VideoID, &rep.Reason, &rep.Status, &rep.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rep)
	}
	return out, rows.Err()
}

func (r *engagementsRepo) ReportResolve(ctx context.Context, id int64, status string) error {
	_, err := r.pg.Exec(ctx, `UPDATE reports SET status=$2 WHERE id=$1`, id, status)
	return err
}

func (r *engagementsRepo) RecordView(ctx context.Context, userID, videoID int64, watchSeconds int) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO views_log (user_id, video_id, watch_seconds)
		VALUES ($1,$2,$3)`, userID, videoID, watchSeconds)
	return err
}

func (r *engagementsRepo) CountViews(ctx context.Context, videoID int64) (int64, error) {
	var n int64
	err := r.pg.QueryRow(ctx, `SELECT count(*) FROM views_log WHERE video_id=$1`, videoID).Scan(&n)
	return n, err
}

var _ Engagements = (*engagementsRepo)(nil)