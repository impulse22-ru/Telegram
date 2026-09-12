package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type channelsRepo struct{ pg *pgxpool.Pool }

// EnsureByTgChatID возвращает id канала, создавая запись при отсутствии.
func (r *channelsRepo) EnsureByTgChatID(ctx context.Context, tgChatID int64, kind, title string) (int64, error) {
	var id int64
	if kind == "" {
		kind = "feed"
	}
	err := r.pg.QueryRow(ctx, `
		INSERT INTO channels (tg_chat_id, title, kind)
		VALUES ($1, $2, $3)
		ON CONFLICT (tg_chat_id) DO UPDATE SET
			title = COALESCE(NULLIF(EXCLUDED.title, ''), channels.title)
		RETURNING id`, tgChatID, title, kind).Scan(&id)
	return id, err
}

func (r *channelsRepo) GetByTgChatID(ctx context.Context, tgChatID int64) (*Channel, error) {
	var ch Channel
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_chat_id, COALESCE(title,''), kind, is_private, COALESCE(owner_id,0), created_at
		FROM channels WHERE tg_chat_id=$1`, tgChatID).
		Scan(&ch.ID, &ch.TgChatID, &ch.Title, &ch.Kind, &ch.IsPrivate, &ch.OwnerID, &ch.CreatedAt)
	if err != nil {
		return nil, err
	}
	return &ch, nil
}

var _ Channels = (*channelsRepo)(nil)