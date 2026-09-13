package store

import (
	"context"
	"strconv"

	"github.com/redis/go-redis/v9"
)

// feedRepo — лента на Redis ZSET.
// Ключ: feed:{channelID}; score = unix (новые выше/меньше по score при ZREVRANGE).
// Дубликаты устраняются INCR по score (появляются самые свежие).
type feedRepo struct{ rdb *redis.Client }

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

func (r *feedRepo) RemoveVideo(ctx context.Context, channelID, videoID int64) error {
	key := feedKey(channelID)
	return r.rdb.ZRem(ctx, key, videoID).Err()
}

func feedKey(channelID int64) string {
	return "feed:" + strconv.FormatInt(channelID, 10)
}