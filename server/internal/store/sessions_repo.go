package store

import (
	"context"
	"strconv"
	"time"

	"github.com/redis/go-redis/v9"

	"tgcloud/server/internal/auth"
)

const refreshKeyPrefix = "refresh:"

// sessionsRepo — реализация auth.RefreshStore поверх Redis.
// Ключ: refresh:<sha256-raw>, значение: userID (int64), TTL = время жизни refresh.
type sessionsRepo struct {
	rdb *redis.Client
}

// SaveRefresh — сохраняет refresh-сессию с заданным TTL.
func (s *sessionsRepo) SaveRefresh(ctx context.Context, refreshHash string, userID int64, ttl time.Duration) error {
	return s.rdb.Set(ctx, refreshKeyPrefix+refreshHash, userID, ttl).Err()
}

// RefreshUserID — возвращает id пользователя по hash refresh-токена;
// ErrNoRefresh если ключ отсутствует (истёк / отзыв / невалидный токен).
func (s *sessionsRepo) RefreshUserID(ctx context.Context, refreshHash string) (int64, error) {
	v, err := s.rdb.Get(ctx, refreshKeyPrefix+refreshHash).Result()
	if err == redis.Nil {
		return 0, auth.ErrNoRefresh
	}
	if err != nil {
		return 0, err
	}
	uid, err := strconv.ParseInt(v, 10, 64)
	if err != nil {
		return 0, auth.ErrNoRefresh
	}
	return uid, nil
}

// DeleteRefresh — отзывает refresh-токен (удаляет сессию из Redis).
func (s *sessionsRepo) DeleteRefresh(ctx context.Context, refreshHash string) error {
	return s.rdb.Del(ctx, refreshKeyPrefix+refreshHash).Err()
}

// Проверка соответствия интерфейсу auth.SessionStore.
var _ auth.SessionStore = (*sessionsRepo)(nil)

// NewSessionStore — конструктор; store уже содержит redis.Client.
func NewSessionStore(rdb *redis.Client) auth.SessionStore {
	return &sessionsRepo{rdb: rdb}
}
