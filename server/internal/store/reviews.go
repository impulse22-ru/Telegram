package store

import (
	"context"

	"github.com/jackc/pgx/v5/pgxpool"
)

type reviewsRepo struct{ pg *pgxpool.Pool }

func (r *reviewsRepo) Add(ctx context.Context, rev Review) error {
	_, err := r.pg.Exec(ctx, `
		INSERT INTO reviews (user_id, product_id, rating, text)
		VALUES ($1, $2, $3, $4)
		ON CONFLICT (user_id, product_id) DO UPDATE SET rating = $3, text = $4`,
		rev.UserID, rev.ProductID, rev.Rating, rev.Text)
	return err
}

func (r *reviewsRepo) ByProduct(ctx context.Context, productID int64) ([]Review, error) {
	rows, err := r.pg.Query(ctx, `
		SELECT id, user_id, product_id, rating, COALESCE(text,''), created_at
		FROM reviews WHERE product_id=$1 ORDER BY id DESC`, productID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var out []Review
	for rows.Next() {
		var rev Review
		if err := rows.Scan(&rev.ID, &rev.UserID, &rev.ProductID, &rev.Rating, &rev.Text, &rev.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, rev)
	}
	return out, rows.Err()
}

func (r *reviewsRepo) AvgRating(ctx context.Context, productID int64) (float64, int64, error) {
	var avg float64
	var count int64
	err := r.pg.QueryRow(ctx, `
		SELECT COALESCE(AVG(rating), 0), COUNT(*)
		FROM reviews WHERE product_id=$1`, productID).Scan(&avg, &count)
	return avg, count, err
}

var _ Reviews = (*reviewsRepo)(nil)
