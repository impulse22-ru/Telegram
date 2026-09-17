package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

// videosRepo — реализация Videos на pgxpool.
type videosRepo struct{ pg *pgxpool.Pool }

// Insert — добавляет видео; ON CONFLICT по (channel_id, tg_msg_id) игнорирует дубликаты.
func (r *videosRepo) Insert(ctx context.Context, v Video) error {
	tags := v.Tags
	if tags == nil {
		tags = []string{}
	}
	_, err := r.pg.Exec(ctx, `
		INSERT INTO videos (tg_msg_id, file_id, file_unique_id, content_signature, content_hash, caption, duration_ms, width, height, title, tags, channel_id, posted_at)
		VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11,$12,$13)
		ON CONFLICT (channel_id, tg_msg_id) DO NOTHING`,
		v.TgMsgID, v.FileID, v.FileUniqueID, v.ContentSignature, v.ContentHash,
		v.Caption, v.DurationMs, v.Width, v.Height, v.Title, tags, v.ChannelID, v.PostedAt)
	return err
}

// FindBySignature ищет видимое видео с такой же сигнатурой метаданных.
// Сигнатура строится из (duration_ms, width, height, file_size) — перезалитые
// копии одного файла почти всегда совпадают по этим полям.
func (r *videosRepo) FindBySignature(ctx context.Context, signature string) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos
		WHERE status='visible' AND content_signature=$1
		ORDER BY posted_at
		LIMIT 1`, signature).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.FileUniqueID, &v.ContentSignature, &v.ContentHash,
			&v.Caption, &v.DurationMs, &v.Width, &v.Height, &v.Title, &v.Tags, &v.Status,
			&v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// FindByFileUniqueID ищет видимое видео с таким же стабильным ID файла Telegram.
// Это точная копия: даже перезалитый файл имеет тот же file_unique_id.
func (r *videosRepo) FindByFileUniqueID(ctx context.Context, fileUniqueID string) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos
		WHERE status='visible' AND file_unique_id=$1
		ORDER BY posted_at
		LIMIT 1`, fileUniqueID).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.FileUniqueID, &v.ContentSignature, &v.ContentHash,
			&v.Caption, &v.DurationMs, &v.Width, &v.Height, &v.Title, &v.Tags, &v.Status,
			&v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// FindByContentHash ищет видимое видео с таким же SHA-256 содержимого.
// Используется relay: после скачивания файла в кэш хэш можно сравнить точно,
// даже если метаданные были изменены (перекодирование в другой размер).
func (r *videosRepo) FindByContentHash(ctx context.Context, hash string) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos
		WHERE status='visible' AND content_hash=$1
		ORDER BY posted_at
		LIMIT 1`, hash).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.FileUniqueID, &v.ContentSignature, &v.ContentHash,
			&v.Caption, &v.DurationMs, &v.Width, &v.Height, &v.Title, &v.Tags, &v.Status,
			&v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// SetDuplicate помечает видео как дубликат (status='duplicate').
// Такое видео скрывается из видимых лент: VisibleFrom ищет только 'visible'.
func (r *videosRepo) SetDuplicate(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='duplicate' WHERE id=$1`, id)
	return err
}

// SetContentHash сохраняет SHA-256 содержимого видео.
// Вызывается relay-ом после скачивания файла в кэш (fillCache).
func (r *videosRepo) SetContentHash(ctx context.Context, id int64, hash string) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET content_hash=$1 WHERE id=$2`, hash, id)
	return err
}

// VisibleFrom — пагинированный список видимых видео канала, начиная с id > after (курсорная пагинация).
func (r *videosRepo) VisibleFrom(ctx context.Context, channelID, after, limit int64) ([]Video, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos
		WHERE channel_id = $1 AND status = 'visible' AND id > $2
		ORDER BY id
		LIMIT $3`, channelID, after, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	return scanVideos(rows)
}

// Ban — банит видео (меняет статус на 'banned'), скрывая из ленты.
func (r *videosRepo) Ban(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='banned' WHERE id=$1`, id)
	return err
}

// Unban — разбанивает видео (возвращает статус 'visible').
func (r *videosRepo) Unban(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='visible' WHERE id=$1`, id)
	return err
}

// Delete — soft-удаляет видео (меняет статус на 'deleted', физически остаётся в БД).
func (r *videosRepo) Delete(ctx context.Context, id int64) error {
	_, err := r.pg.Exec(ctx, `UPDATE videos SET status='deleted' WHERE id=$1`, id)
	return err
}

// Search — поиск по title/caption (для команды !search в боте).
func (r *videosRepo) Search(ctx context.Context, q string, limit int64) ([]Video, error) {
	if limit <= 0 {
		limit = 10
	}
	// ILIKE с %...% — регистронезависимый поиск по подстроке.
	pattern := "%" + q + "%"
	rows, err := r.pg.Query(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
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

// Count — число видео по статусу (” = всего).
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

// Get — возвращает видео по ID.
func (r *videosRepo) Get(ctx context.Context, id int64) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos WHERE id=$1`, id).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.FileUniqueID, &v.ContentSignature, &v.ContentHash,
			&v.Caption, &v.DurationMs, &v.Width, &v.Height, &v.Title, &v.Tags, &v.Status,
			&v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// GetByTgMsg — возвращает видео по (channel_id, tg_msg_id) — уникальной паре из Telegram.
func (r *videosRepo) GetByTgMsg(ctx context.Context, channelID, tgMsgID int64) (*Video, error) {
	var v Video
	err := r.pg.QueryRow(ctx, `
		SELECT id, tg_msg_id, file_id, COALESCE(file_unique_id,''), COALESCE(content_signature,''), COALESCE(content_hash,''),
		       COALESCE(caption,''), COALESCE(duration_ms,0), COALESCE(width,0), COALESCE(height,0),
		       COALESCE(title,''), tags, status, channel_id, posted_at
		FROM videos WHERE channel_id=$1 AND tg_msg_id=$2`, channelID, tgMsgID).
		Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.FileUniqueID, &v.ContentSignature, &v.ContentHash,
			&v.Caption, &v.DurationMs, &v.Width, &v.Height, &v.Title, &v.Tags, &v.Status,
			&v.ChannelID, &v.PostedAt)
	if err != nil {
		return nil, err
	}
	return &v, nil
}

// scanVideos — общий сканер строк в срез Video; используется Search и VisibleFrom.
func scanVideos(rows interface {
	Next() bool
	Scan(...any) error
	Err() error
	Close()
}) ([]Video, error) {
	out := make([]Video, 0)
	for rows.Next() {
		var v Video
		if err := rows.Scan(&v.ID, &v.TgMsgID, &v.FileID, &v.FileUniqueID, &v.ContentSignature, &v.ContentHash,
			&v.Caption, &v.DurationMs, &v.Width, &v.Height, &v.Title, &v.Tags, &v.Status,
			&v.ChannelID, &v.PostedAt); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

var _ Videos = (*videosRepo)(nil)
