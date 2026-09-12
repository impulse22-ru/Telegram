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

func main() {
	cfg := config.Load()
	if err := run(cfg); err != nil {
		log.Errorf("api terminated: %v", err)
		os.Exit(1)
	}
}

func run(cfg config.Config) error {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	st, err := store.New(ctx, cfg.PGDSN, cfg.RedisAddr)
	if err != nil {
		return err
	}
	defer st.Close()

	am := auth.NewManager(cfg.JWTSecret, cfg.JWTExpiry)
	var bot *tgbot.Client
	if cfg.BotToken != "" {
		bot = tgbot.New(cfg.BotAPIBase, cfg.BotToken)
	}
	srv := httpapi.New(store.NewRepos(st), am, cfg.FeedChID, bot)

	httpSrv := &http.Server{
		Addr:              cfg.HTTPAddr,
		Handler:           srv.Routes(),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Infof("api listening on %s", cfg.HTTPAddr)
		if err := httpSrv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Errorf("http: %v", err)
		}
	}()

	<-ctx.Done()
	log.Infof("shutting down...")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	return httpSrv.Shutdown(shutdownCtx)
}