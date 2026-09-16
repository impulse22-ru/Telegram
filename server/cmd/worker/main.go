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
//
// Протокол long polling:
//  1. Отправляем getUpdates с offset (порядковый номер первого обрабатываемого update).
//  2. Telegram «держит» соединение до Timeout (30 сек) или до появления нового update.
//  3. Получаем пакет updates, обрабатываем каждый, сдвигаем offset.
//  4. При ошибке — повторяем запрос (бэкпофф не нужен, Telegram сам throttle'ит).
//
// Типы обрабатываемых обновлений:
//   - channel_post (из целевого канала feedChat) → видео индексируются в БД.
//   - message (личные сообщения боту) → обработка команд (!search, !stat и т.д.).
//   - chat_join_request (закрытый канал) → автоматическое одобрение вступления.
func main() {
	// Контекст с обработкой SIGINT/SIGTERM — при сигнале цикл polling завершается.
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg := config.Load()

	// Подключение к БД (PostgreSQL) и кэшу (Redis).
	s, err := store.New(ctx, cfg.PGDSN, cfg.RedisAddr)
	if err != nil {
		log.Fatalf("store: %v", err)
	}
	defer s.Close()

	// Без BOT_TOKEN невозможно опросить getUpdates, без FeedChID — неизвестно, какой канал индексировать.
	if cfg.BotToken == "" || cfg.FeedChID == 0 {
		log.Fatalf("worker: BOT_TOKEN и FEED_CHAT_ID обязательны (закрытый канал)")
	}

	// Клиент Bot API: обёртка над HTTP-запросами к /bot<token>/<method>.
	bot := tgbot.New(cfg.BotAPIBase, cfg.BotToken)

	// Проверяем доступность Bot API вызовом getMe — это быстрая health-check операция.
	me, err := bot.GetMe()
	if err != nil {
		log.Fatalf("worker: Bot API недоступен: %v", err)
	}
	log.Printf("worker: bot %s активен, индексирую канал %d", me.Username, cfg.FeedChID)

	// Indexer — диспетчер: маршрутизирует update на нужный обработчик.
	ix := indexer.New(bot, store.NewRepos(s), cfg.FeedChID)

	// offset — номер первого необработанного update.
	// Telegram возвращает updates с update_id >= offset; после обработки сдвигаем.
	var offset int
	for {
		// Timeout=30: long polling — соединение «висит» 30 сек, если новых нет.
		// Limit=100: максимум updates за один запрос (Telegram лимит — 100).
		updates, err := bot.GetUpdates(tgbot.GetUpdatesReq{Offset: offset, Timeout: 30, Limit: 100})
		if err != nil {
			// При сетевой ошибке — повторяем; Telegram автоматически throttle'ит при частых ошибках.
			log.Printf("worker: getUpdates: %v (retry)", err)
			continue
		}
		for _, u := range updates {
			if ix.Handle(ctx, &u) == indexer.Handled {
				log.Printf("worker: handled update %d", u.UpdateID)
			}
			// Сдвигаем offset: следующий запрос начнётся с этого update_id + 1.
			offset = int(u.UpdateID) + 1
		}
	}
}
