package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tgcloud/server/internal/config"
	"tgcloud/server/internal/media"
	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// relay — медиа-прокси стриминга (видео из Telegram CDN → клиент).
// Этап 3: HTTP Range + дисковый кэш.
//
// Архитектура:
//   - Клиент запрашивает GET /media/stream/{video_id}.
//   - Relay проверяет локальный кэш (media_cache/video_<id>.mp4).
//   - Если файла нет — скачивает из Telegram Bot API (getFile → /file/bot<token>/<path>).
//   - Отдаёт файл через http.ServeContent с поддержкой HTTP Range (байтовая навигация).
//   - Дисковый кэш ускоряет повторные запросы и позволяет делать перемотку без повторного скачивания.
func main() {
	// Контекст с обработкой SIGINT/SIGTERM для корректного завершения.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	// Подключение к PostgreSQL + Redis (нужны для поиска video по ID).
	st, err := store.New(ctx, cfg.PGDSN, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer st.Close()

	// BOT_TOKEN обязателен — без него невозможно скачать файл из Telegram CDN.
	if cfg.BotToken == "" {
		log.Fatalf("relay: BOT_TOKEN обязателен")
	}
	bot := tgbot.New(cfg.BotAPIBase, cfg.BotToken)

	// Relay — ядро: связывает store (поиск видео), bot (скачивание) и кэш (диск).
	rl := media.New(store.NewRepos(st), bot, cfg.RelayCacheDir)

	mux := http.NewServeMux()

	// Healthcheck для Kubernetes/Docker — 200 OK, если процесс жив.
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("relay ok"))
	})

	// /metrics — экспорт счётчиков relay в Prometheus text format (без авторизации).
	mux.HandleFunc("GET /metrics", func(w http.ResponseWriter, r *http.Request) {
		m := rl.Metrics()
		w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
		_, _ = fmt.Fprintf(w,
			"# HELP relay_streams_total Requests to stream endpoint.\n"+
				"# TYPE relay_streams_total counter\nrelay_streams_total %d\n"+
				"# TYPE relay_bytes_total counter\nrelay_bytes_total %d\n"+
				"# TYPE relay_cache_hits_total counter\nrelay_cache_hits_total %d\n"+
				"# TYPE relay_not_found_total counter\nrelay_not_found_total %d\n",
			m.Streams(), m.Bytes(), m.CacheHits(), m.NotFound())
	})

	// Основной роут: стриминг видео по его внутреннему ID.
	// {video_id} — path parameter (Go 1.22+ routing).
	mux.HandleFunc("GET /media/stream/{video_id}", media.RelayHandler(rl))

	// Запуск HTTP-сервера.
	addr := ":" + cfg.RelayPort
	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("relay listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("relay http: %v", err)
		}
	}()

	// Ожидание сигнала завершения → graceful shutdown.
	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}
