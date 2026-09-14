package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type statsRepo struct{ pg *pgxpool.Pool }

// VideoViewsDay — просмотры видео по дням за последние N дней.
func (r *statsRepo) VideoViewsDay(ctx context.Context, videoID int64, days int) ([]ViewDay, error) {
	if days <= 0 {
		days = 7
	}
	rows, err := r.pg.Query(ctx, `
		SELECT date_trunc('day', created_at)::date AS day, count(*) AS views, count(DISTINCT user_id) AS uniques
		FROM views_log
		WHERE video_id=$1 AND created_at >= now() - ($2::int * interval '1 day')
		GROUP BY day ORDER BY day`, videoID, days)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]ViewDay, 0)
	for rows.Next() {
		var v ViewDay
		if err := rows.Scan(&v.Day, &v.Views, &v.Uniques); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

// VideoStat — агрегированная статистика по одному видео.
func (r *statsRepo) VideoStat(ctx context.Context, videoID int64) (*VideoStat, error) {
	var v VideoStat
	err := r.pg.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM views_log vl WHERE vl.video_id=$1) AS views,
			(SELECT count(DISTINCT user_id) FROM views_log vl WHERE vl.video_id=$1) AS uniques,
			(SELECT count(*) FROM likes l WHERE l.video_id=$1) AS likes,
			COALESCE((SELECT avg(watch_seconds)::float8 FROM views_log vl WHERE vl.video_id=$1), 0) AS avg_watch
		`, videoID).Scan(&v.Views, &v.Uniques, &v.Likes, &v.AvgWatchSeconds)
	if err != nil {
		return nil, err
	}
	v.VideoID = videoID
	return &v, nil
}

// TopVideos — топ видео по суммарным просмотрам.
func (r *statsRepo) TopVideos(ctx context.Context, limit int64) ([]VideoStat, error) {
	if limit <= 0 {
		limit = 10
	}
	rows, err := r.pg.Query(ctx, `
		SELECT video_id, count(*) AS views, count(DISTINCT user_id) AS uniques,
		       COALESCE(avg(watch_seconds),0)::float8 AS avg_watch
		FROM views_log
		GROUP BY video_id ORDER BY views DESC LIMIT $1`, limit)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	out := make([]VideoStat, 0, limit)
	for rows.Next() {
		var v VideoStat
		if err := rows.Scan(&v.VideoID, &v.Views, &v.Uniques, &v.AvgWatchSeconds); err != nil {
			return nil, err
		}
		v.Likes, _ = r.countLikes(ctx, v.VideoID)
		out = append(out, v)
	}
	return out, rows.Err()
}

func (r *statsRepo) countLikes(ctx context.Context, videoID int64) (int64, error) {
	var n int64
	err := r.pg.QueryRow(ctx, `SELECT count(*) FROM likes WHERE video_id=$1`, videoID).Scan(&n)
	return n, err
}

// UserStats — персональная статистика пользователя.
func (r *statsRepo) UserStats(ctx context.Context, userID int64) (*UserStats, error) {
	var s UserStats
	err := r.pg.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM views_log WHERE user_id=$1),
			(SELECT COALESCE(sum(watch_seconds),0)::bigint FROM views_log WHERE user_id=$1),
			(SELECT count(*) FROM likes WHERE user_id=$1),
			(SELECT count(*) FROM comments WHERE user_id=$1),
			(SELECT count(*) FROM subscriptions WHERE user_id=$1)`,
		userID).Scan(&s.Views, &s.WatchSeconds, &s.LikesGiven, &s.CommentsGiven, &s.Subscriptions)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// AdminStats — общий дашборд.
func (r *statsRepo) AdminStats(ctx context.Context) (*AdminStats, error) {
	var s AdminStats
	err := r.pg.QueryRow(ctx, `
		SELECT
			(SELECT count(*) FROM users) AS users,
			(SELECT count(*) FROM videos) AS videos,
			(SELECT count(*) FROM videos WHERE status='visible') AS visible,
			(SELECT count(*) FROM views_log) AS views,
			(SELECT count(*) FROM likes) AS likes,
			(SELECT count(*) FROM comments) AS comments,
			(SELECT count(*) FROM reports WHERE status='open') AS reports_open,
			(SELECT count(*) FROM shops WHERE status='active') AS shops,
			(SELECT count(*) FROM products WHERE status='on_sale') AS products,
			(SELECT count(*) FROM orders) AS orders,
			COALESCE((SELECT sum(price_amount) FROM orders WHERE payment_status='confirmed'),0)::float8 AS revenue
		`).Scan(&s.Users, &s.Videos, &s.VisibleVideos, &s.Views, &s.Likes,
		&s.Comments, &s.ReportsOpen, &s.Shops, &s.Products, &s.Orders, &s.Revenue)
	if err != nil {
		return nil, err
	}
	return &s, nil
}

// SellerStats — аналитика продавца: заказы, выручка, топ товаров.
func (r *statsRepo) SellerStats(ctx context.Context, ownerID int64) (*SellerStats, error) {
	var s SellerStats
	err := r.pg.QueryRow(ctx, `
		SELECT
			(SELECT count(o.*) FROM orders o JOIN shops sh ON sh.id=o.shop_id WHERE sh.owner_id=$1) AS orders,
			(SELECT count(o.*) FROM orders o JOIN shops sh ON sh.id=o.shop_id WHERE sh.owner_id=$1 AND o.payment_status='pending') AS pending,
			(SELECT count(o.*) FROM orders o JOIN shops sh ON sh.id=o.shop_id WHERE sh.owner_id=$1 AND o.payment_status='confirmed') AS confirmed,
			COALESCE((SELECT sum(o.price_amount) FROM orders o JOIN shops sh ON sh.id=o.shop_id WHERE sh.owner_id=$1 AND o.payment_status='confirmed'),0)::float8 AS revenue,
			COALESCE((SELECT count(pv.*) FROM product_views pv JOIN products p ON p.id=pv.product_id JOIN shops sh ON sh.id=p.shop_id WHERE sh.owner_id=$1),0) AS views
		`, ownerID).Scan(&s.Orders, &s.Pending, &s.Confirmed, &s.Revenue, &s.Views)
	if err != nil {
		return nil, err
	}

	rows, err := r.pg.Query(ctx, `
		SELECT p.id, p.title,
			COALESCE((SELECT count(*) FROM product_views pv WHERE pv.product_id=p.id),0) AS views,
			(SELECT count(*) FROM orders o WHERE o.product_id=p.id) AS orders,
			COALESCE((SELECT sum(o.price_amount) FROM orders o WHERE o.product_id=p.id AND o.payment_status='confirmed'),0)::float8 AS revenue
		FROM products p JOIN shops sh ON sh.id=p.shop_id
		WHERE sh.owner_id=$1
		ORDER BY revenue DESC, views DESC
		LIMIT 20`, ownerID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	for rows.Next() {
		var p ProductStat
		if err := rows.Scan(&p.ProductID, &p.Title, &p.Views, &p.Orders, &p.Revenue); err != nil {
			return nil, err
		}
		s.ByProduct = append(s.ByProduct, p)
	}
	return &s, rows.Err()
}

type subscriptionsRepo struct{ pg *pgxpool.Pool }

func (r *subscriptionsRepo) Subscribe(ctx context.Context, userID, channelID int64) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO subscriptions (user_id, channel_id) VALUES ($1,$2)
		ON CONFLICT (user_id, channel_id) DO NOTHING`, userID, channelID)
	return err
}

func (r *subscriptionsRepo) Unsubscribe(ctx context.Context, userID, channelID int64) error {
	_, err := r.pg.Exec(ctx, `DELETE FROM subscriptions WHERE user_id=$1 AND channel_id=$2`, userID, channelID)
	return err
}

func (r *subscriptionsRepo) IsSubscribed(ctx context.Context, userID, channelID int64) (bool, error) {
	var exists bool
	err := r.pg.QueryRow(ctx, `SELECT EXISTS(SELECT 1 FROM subscriptions WHERE user_id=$1 AND channel_id=$2)`, userID, channelID).Scan(&exists)
	return exists, err
}

func (r *subscriptionsRepo) ByUser(ctx context.Context, userID int64) ([]int64, error) {
	rows, err := r.pg.Query(ctx, `SELECT channel_id FROM subscriptions WHERE user_id=$1 ORDER BY created_at DESC`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]int64, 0)
	for rows.Next() {
		var id int64
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

type filterRepo struct{ pg *pgxpool.Pool }

func (r *filterRepo) Add(ctx context.Context, word string) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO filter_words (word) VALUES ($1)
		ON CONFLICT (word) DO NOTHING`, word)
	return err
}

func (r *filterRepo) List(ctx context.Context) ([]string, error) {
	rows, err := r.pg.Query(ctx, `SELECT word FROM filter_words ORDER BY id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]string, 0)
	for rows.Next() {
		var w string
		if err := rows.Scan(&w); err != nil {
			return nil, err
		}
		out = append(out, w)
	}
	return out, rows.Err()
}

func (r *filterRepo) Remove(ctx context.Context, word string) error {
	_, err := r.pg.Exec(ctx, `DELETE FROM filter_words WHERE word=$1`, word)
	return err
}

var (
	_ Stats         = (*statsRepo)(nil)
	_ Subscriptions = (*subscriptionsRepo)(nil)
	_ Filter        = (*filterRepo)(nil)
)