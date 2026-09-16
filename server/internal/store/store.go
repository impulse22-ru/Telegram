package store

import (
	"context"
	"fmt"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/redis/go-redis/v9"

	"tgcloud/server/internal/log"
)

// Store — хранилище данных: пул соединений PostgreSQL + клиент Redis.
type Store struct {
	PG    *pgxpool.Pool
	Redis *redis.Client
}

// New — инициализирует Store: подключается к PG и Redis, проверяет связь через Ping.
func New(ctx context.Context, pgDSN, redisAddr string) (*Store, error) {
	pg, err := pgxpool.New(ctx, pgDSN)
	if err != nil {
		return nil, fmt.Errorf("pg connect: %w", err)
	}
	// Ping PG с таймаутом 5 сек, чтобы не висеть бесконечно при недоступности БД.
	ctxPing, cancel := context.WithTimeout(ctx, 5*time.Second)
	defer cancel()
	if err := pg.Ping(ctxPing); err != nil {
		return nil, fmt.Errorf("pg ping: %w", err)
	}

	rdb := redis.NewClient(&redis.Options{Addr: redisAddr})
	// Ping Redis с таймаутом 5 сек — аналогичная проверка доступности.
	ctxPing2, cancel2 := context.WithTimeout(ctx, 5*time.Second)
	defer cancel2()
	if err := rdb.Ping(ctxPing2).Err(); err != nil {
		return nil, fmt.Errorf("redis ping: %w", err)
	}

	log.Infof("store connected: pg + redis")
	return &Store{PG: pg, Redis: rdb}, nil
}

// Close — корректно закрывает пул PG и клиент Redis.
func (s *Store) Close() {
	s.PG.Close()
	_ = s.Redis.Close()
}
