package main

import (
	"context"
	"errors"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tgcloud/server/internal/auth"
	"tgcloud/server/internal/config"
	"tgcloud/server/internal/httpapi"
	"tgcloud/server/internal/log"
	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// main — точка входа API-сервера.
// Загружает конфигурацию из переменных окружения и запускает HTTP-сервер.
// При фатальной ошибке завершается с кодом 1.
func main() {
	cfg := config.Load()
	if err := run(cfg); err != nil {
		log.Errorf("api terminated: %v", err)
		os.Exit(1)
	}
}

// run — основной цикл жизни API-сервера.
// Выполняет последовательную инициализацию всех компонентов:
//  1. Создаёт контекст с обработкой SIGINT/SIGTERM для graceful shutdown.
//  2. Подключается к PostgreSQL + Redis через store.New.
//  3. Создаёт JWT-менеджер (auth.Manager) для выдачи/проверки токенов.
//  4. Опционально инициализирует клиент Telegram Bot API (если BOT_TOKEN задан).
//  5. Собирает HTTP-сервер с роутами, переданными из httpapi.
//  6. Запускает ListenAndServe в отдельной горутине.
//  7. Блокируется на <-ctx.Done() до получения сигнала завершения,
//     затем выполняет Shutdown с таймаутом 10 секунд для завершения
//     активных HTTP-соединений.
//
// Возвращает ошибку, если store или httpapi не удалось инициализировать.
func run(cfg config.Config) error {
	// Контекст, отменяемый при SIGINT (Ctrl+C) или SIGTERM (kill).
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// Подключение к БД (PostgreSQL) и кэшу (Redis).
	st, err := store.New(ctx, cfg.PGDSN, cfg.RedisAddr)
	if err != nil {
		return err
	}
	defer st.Close()

	// JWT-менеджер: secret — HMAC-ключ для подписи, expiry — время жизни access-токена.
	am := auth.NewManager(cfg.JWTSecret, cfg.JWTExpiry)
	// Привязываем refresh-сессии (Redis): opaque-токены с ротацией и отзывом.
	am.WithRefresh(cfg.JWTRefreshExpiry, store.NewSessionStore(st.Redis))

	// Клиент Telegram Bot API: опциональный, нужен для команд бота.
	var bot *tgbot.Client
	if cfg.BotToken != "" {
		bot = tgbot.New(cfg.BotAPIBase, cfg.BotToken)
	}

	// HTTPAPI — фасад: собирает все хендлеры (роуты, middleware, static).
	srv := httpapi.New(store.NewRepos(st), am, cfg.FeedChID, bot, cfg.UploadDir, cfg.HTTPAddr)

	// ReadHeaderTimeout защищает от slowloris-атак (клиенты, не отправляющие заголовки).
	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	// Запуск HTTP-сервера в фоновой горутине; ошибки логируются,
	// кроме ErrServerClosed — это штатное завершение при Shutdown.
	go func() {
		log.Infof("api listening on %s", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("http: %v", err)
		}
	}()

	// Блокируемся до получения сигнала (SIGINT/SIGTERM).
	<-ctx.Done()
	log.Infof("shutting down...")

	// Graceful shutdown: 10 секунд на завершение активных запросов.
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}
