package main

import (
	"context"
	"os"
	"os/signal"
	"syscall"
	"time"

	"tgcloud/server/internal/config"
	"tgcloud/server/internal/log"
	"tgcloud/server/internal/store"
)

// worker — фоновая индексация канала через локальный Bot API.
// Этап 0-1: точка входа и цикл опроса готовы; наполнение — этап 1 (бот).
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()
	st, err := store.New(ctx, cfg.PGDSN, cfg.RedisAddr)
	if err != nil {
		log.Errorf("worker: %v", err)
		os.Exit(1)
	}
	defer st.Close()

	log.Infof("worker started (feed_chat_id=%d, bot_api=%s)", cfg.FeedChID, cfg.BotAPIBase)

	ticker := time.NewTicker(30 * time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			log.Infof("worker stopped")
			return
		case <-ticker.C:
			log.Infof("worker tick — poll pending (бот-индексация появится на этапе 1)")
		}
	}
}