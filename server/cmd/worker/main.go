package main

import (
	"context"
	"log"
	"os"
	"os/signal"
	"syscall"

	"tgcloud/server/internal/config"
	"tgcloud/server/internal/indexer"
	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// worker — фоновая индексация ленточного канала.
// Через локальный Bot API: getUpdates (long polling),
// channel_post → видео в БД, chat_join_request → автодобавление.
func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	s, err := store.New(ctx, cfg.PGDSN, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	if cfg.BotToken == "" || cfg.FeedChID == 0 {
		log.Fatalf("worker: BOT_TOKEN и FEED_CHAT_ID обязательны (закрытый канал)")
	}

	bot := tgbot.New(cfg.BotAPIBase, cfg.BotToken)
	me, err := bot.GetMe()
	if err != nil {
		log.Fatalf("worker: Bot API недоступен: %v", err)
	}
	log.Printf("worker: bot %s активен, индексирую канал %d", me.Username, cfg.FeedChID)

	ix := indexer.New(bot, store.NewRepos(s), cfg.FeedChID)

	var offset int
	for {
		updates, err := bot.GetUpdates(tgbot.GetUpdatesReq{Offset: offset, Timeout: 30, Limit: 100})
		if err != nil {
			log.Printf("worker: getUpdates: %v (retry)", err)
			continue
		}
		for _, u := range updates {
			if ix.Handle(ctx, &u) == indexer.Handled {
				log.Printf("worker: handled update %d", u.UpdateID)
			}
			offset = int(u.UpdateID) + 1
		}
	}
}