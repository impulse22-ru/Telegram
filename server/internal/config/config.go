package config

import (
	"os"
	"strconv"
	"time"
)

type Config struct {
	Env        string
	HTTPAddr   string
	PGDSN      string
	RedisAddr  string
	JWTSecret  string
	JWTExpiry  time.Duration
	BotAPIBase string // http://localhost:8081
	BotToken   string
	FeedChID   int64 // закрытый канал ленты (tg_chat_id)
}

func Load() Config {
	return Config{
		Env:       get("APP_ENV", "dev"),
		HTTPAddr:  ":" + get("HTTP_PORT", "8080"),
		PGDSN:     get("PG_DSN", "postgres://tg:tg@localhost:5432/tiktok?sslmode=disable"),
		RedisAddr: get("REDIS_ADDR", "localhost:6379"),
		JWTSecret: get("JWT_SECRET", "dev-secret-change-me"),
		JWTExpiry: getDur("JWT_EXPIRY", 15*time.Minute),
		BotAPIBase: get("BOT_API_BASE", "http://localhost:8081"),
		BotToken:  get("BOT_TOKEN", ""),
		FeedChID:  getInt64("FEED_CHAT_ID", 0),
	}
}

func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

func getDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

func getInt64(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}