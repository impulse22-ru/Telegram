package config

import (
	"os"
	"strconv"
	"time"
)

// Config — структура конфигурации сервера.
// Все поля загружаются из переменных окружения с дефолтными значениями
// (см. Load). Для локальной разработки дефолты работают «из коробки».
//
// Поля:
//   - Env: окружение (dev/staging/prod), влияет на уровень логирования.
//   - HTTPAddr: адрес API-сервера (в формате ":8080").
//   - PGDSN: строка подключения PostgreSQL (Data Source Name).
//   - RedisAddr: адрес Redis (host:port) для кэша ленты и rate-limiting.
//   - JWTSecret: HMAC-ключ для подписи JWT (HS256).
//   - JWTExpiry: время жизни access-токена (например, 15 минут).
//   - BotAPIBase: базовый URL локального Bot API (http://localhost:8081).
//   - BotToken: токен Telegram-бота (BotFather).
//   - FeedChID: Telegram chat_id закрытого канала ленты.
//   - RelayPort: порт медиа-прокси (отдельный процесс).
//   - RelayCacheDir: директория для кэша скачанных видео.
//   - UploadDir: директория для загружаемых файлов.
type Config struct {
	Env           string
	HTTPAddr      string
	PGDSN         string
	RedisAddr     string
	JWTSecret     string
	JWTExpiry     time.Duration
	BotAPIBase    string // http://localhost:8081
	BotToken      string
	FeedChID      int64 // закрытый канал ленты (tg_chat_id)
	RelayPort     string
	RelayCacheDir string
	UploadDir     string
}

// Load — загрузка конфигурации из переменных окружения.
//
// Для каждого поля читается ENV-переменная; если она пуста — используется дефолт:
//
//	APP_ENV → "dev"                    — окружение
//	HTTP_PORT → "8080"                 — порт API (префикс ":" добавляется автоматически)
//	PG_DSN → "postgres://tg:tg@localhost:5432/tiktok?sslmode=disable"
//	REDIS_ADDR → "localhost:6379"
//	JWT_SECRET → "dev-secret-change-me" — !сменить в проде!
//	JWT_EXPIRY → 15*time.Minute
//	BOT_API_BASE → "http://localhost:8081" — локальный Bot API (tdlib)
//	BOT_TOKEN → "" — обязателен для worker и relay
//	FEED_CHAT_ID → 0 — ID канала, индексируется worker'ом
//	RELAY_PORT → "8082"
//	RELAY_CACHE_DIR → "./media_cache"
//	UPLOAD_DIR → "./uploads"
//
// Все значения типизированы: строки, int64, time.Duration.
// Ошибки парсинга игнорируются — используется дефолт.
func Load() Config {
	return Config{
		Env:           get("APP_ENV", "dev"),
		HTTPAddr:      ":" + get("HTTP_PORT", "8080"),
		PGDSN:         get("PG_DSN", "postgres://tg:tg@localhost:5432/tiktok?sslmode=disable"),
		RedisAddr:     get("REDIS_ADDR", "localhost:6379"),
		JWTSecret:     get("JWT_SECRET", "dev-secret-change-me"),
		JWTExpiry:     getDur("JWT_EXPIRY", 15*time.Minute),
		BotAPIBase:    get("BOT_API_BASE", "http://localhost:8081"),
		BotToken:      get("BOT_TOKEN", ""),
		FeedChID:      getInt64("FEED_CHAT_ID", 0),
		RelayPort:     get("RELAY_PORT", "8082"),
		RelayCacheDir: get("RELAY_CACHE_DIR", "./media_cache"),
		UploadDir:     get("UPLOAD_DIR", "./uploads"),
	}
}

// get — чтение строковой переменной окружения с дефолтом.
// Если переменная не задана или пуста — возвращает def.
func get(k, def string) string {
	if v := os.Getenv(k); v != "" {
		return v
	}
	return def
}

// getDur — чтение переменной окружения как time.Duration.
// Парсит строку через time.ParseDuration (например, "15m", "1h30m").
// При пустом значении или ошибке парсинга — возвращает def.
func getDur(k string, def time.Duration) time.Duration {
	if v := os.Getenv(k); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return def
}

// getInt64 — чтение переменной окружения как int64.
// Парсит строку как десятичное число (base 10).
// При пустом значении или ошибке парсинга — возвращает def.
func getInt64(k string, def int64) int64 {
	if v := os.Getenv(k); v != "" {
		if n, err := strconv.ParseInt(v, 10, 64); err == nil {
			return n
		}
	}
	return def
}
