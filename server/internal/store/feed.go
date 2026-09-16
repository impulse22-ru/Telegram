package store

import (
	"context"
	"math"
	"strconv"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"
)

// feedRepo — лента на Redis ZSET (кандидаты), скоринг из PG.
type feedRepo struct {
	rdb *redis.Client
	pg  *pgxpool.Pool
}

// AddVideo — добавляет видео в ленту канала с начальным score (для кандидатов в Redis ZSET).
func (r *feedRepo) AddVideo(ctx context.Context, channelID, videoID int64, score float64) error {
	key := feedKey(channelID)
	return r.rdb.ZAdd(ctx, key, redis.Z{Score: score, Member: videoID}).Err()
}

// Top — последние N видео по score (обратный порядок = свежие первыми).
func (r *feedRepo) Top(ctx context.Context, channelID, n int64) ([]int64, error) {
	key := feedKey(channelID)
	members, err := r.rdb.ZRevRange(ctx, key, 0, n-1).Result()
	if err != nil {
		return nil, err
	}
	out := make([]int64, 0, len(members))
	for _, m := range members {
		id, err := strconv.ParseInt(m, 10, 64)
		if err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, nil
}

// ScoredFeed — пагинированная лента с скорингом по формуле плана.
// Кандидаты берутся из Redis (top 200 по времени), скоринг считается из PG.
func (r *feedRepo) ScoredFeed(ctx context.Context, channelID, offset, limit int64) ([]int64, error) {
	if limit <= 0 {
		limit = 20
	}
	// Кандидаты из Redis: берём 200 самых свежих.
	key := feedKey(channelID)
	rawIDs, err := r.rdb.ZRevRange(ctx, key, 0, 199).Result()
	if err != nil {
		return nil, err
	}
	ids := make([]int64, 0, len(rawIDs))
	for _, m := range rawIDs {
		if id, err := strconv.ParseInt(m, 10, 64); err == nil {
			ids = append(ids, id)
		}
	}
	if len(ids) == 0 {
		return nil, nil
	}

	// Скоринг из PG: считаем score для каждого кандидата отдельным запросом.
	type vs struct {
		id    int64
		score float64
	}
	scored := make([]vs, 0, len(ids))
	for _, id := range ids {
		sc, err := r.videoScore(ctx, id)
		if err != nil {
			// Видео могло быть удалено — пропускаем, не прерываем ленту.
			continue
		}
		scored = append(scored, vs{id: id, score: sc})
	}

	// Сортировка по убыванию score (Insertion Sort — достаточно для 200 элементов).
	for i := 1; i < len(scored); i++ {
		for j := i; j > 0 && scored[j].score > scored[j-1].score; j-- {
			scored[j], scored[j-1] = scored[j-1], scored[j]
		}
	}

	// Пагинация по отсортированному массиву.
	start := int(offset)
	if start > len(scored) {
		start = len(scored)
	}
	end := start + int(limit)
	if end > len(scored) {
		end = len(scored)
	}
	out := make([]int64, 0, end-start)
	for _, s := range scored[start:end] {
		out = append(out, s.id)
	}
	return out, nil
}

// videoScore считает score по формуле:
//
//	0.6 * likes_velocity(24h) + 0.25 * comments_velocity(12h)
//	+ 0.15 * view_time_normalized - 0.5 * log(1 + hours_since_upload)
func (r *feedRepo) videoScore(ctx context.Context, videoID int64) (float64, error) {
	var likes24h, comments12h, avgWatch, durationMs, hoursSince float64
	// Один запрос собирает все метрики для формулы скоринга:
	//   - количество лайков за 24ч
	//   - количество комментариев за 12ч
	//   - среднее время просмотра (watch_seconds)
	//   - длительность видео (для нормализации)
	//   - часы с момента загрузки (для штрафа за «старость»)
	err := r.pg.QueryRow(ctx, `
		SELECT
		  (SELECT count(*) FROM likes l
		    WHERE l.video_id=$1 AND l.created_at > now()-interval '24 hours')::float8,
		  (SELECT count(*) FROM comments c
		    WHERE c.video_id=$1 AND c.created_at > now()-interval '12 hours')::float8,
		  COALESCE((SELECT avg(vl.watch_seconds)::float8 FROM views_log vl
		    WHERE vl.video_id=$1), 0),
		  (SELECT duration_ms::float8 FROM videos WHERE id=$1),
		  EXTRACT(EPOCH FROM (now()-(SELECT posted_at FROM videos WHERE id=$1)))/3600.0`,
		videoID).Scan(&likes24h, &comments12h, &avgWatch, &durationMs, &hoursSince)
	if err != nil {
		return 0, err
	}
	// Нормализуем среднее время просмотра к [0, 1] относительно длительности видео.
	viewNorm := 0.0
	if durationMs > 0 {
		viewNorm = avgWatch / (durationMs / 1000.0)
		if viewNorm > 1 {
			viewNorm = 1
		}
	}
	score := 0.6*likes24h + 0.25*comments12h + 0.15*viewNorm - 0.5*math.Log(1+hoursSince)
	return score, nil
}

// RemoveVideo — удаляет видео из Redis ZSET ленты канала.
func (r *feedRepo) RemoveVideo(ctx context.Context, channelID, videoID int64) error {
	key := feedKey(channelID)
	return r.rdb.ZRem(ctx, key, videoID).Err()
}

// feedKey — генерирует Redis-ключ для ZSET ленты канала: "feed:{channelID}".
func feedKey(channelID int64) string {
	return "feed:" + strconv.FormatInt(channelID, 10)
}
