package main

import (
	"context"
	"errors"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tgcloud/server/internal/config"
)

// relay — медиа-прокси стриминга (видео из Telegram CDN → клиент).
// Этап 0-1: HTTP-сервер с заглушкой; наполнение — этап 3.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	addr := cfg.HTTPAddr

	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("relay ok"))
	})
	mux.HandleFunc("GET /media/stream/{video_id}", func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "relay: not implemented (этап 3)", http.StatusNotImplemented)
	})

	srv := &http.Server{Addr: addr, Handler: mux, ReadHeaderTimeout: 5 * time.Second}
	go func() {
		log.Printf("relay listening on %s", addr)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Printf("relay http: %v", err)
		}
	}()

	<-ctx.Done()
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	_ = srv.Shutdown(shutdownCtx)
}