package indexer

import (
	"context"
	"log"
	"strings"
	"time"

	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// Indexer обрабатывает обновления из getUpdates:
//   - channel_post  → индексация видео из ленточного канала в БД
//   - message       → команды бота (!search, !stat, /start, админские !ban и т.п.)
//   - chat_join_request → автоматическое одобрение вступления
//
// Это «мозг» worker-процесса: получает update, маршрутизирует его на
// конкретный обработчик и возвращает результат (Handled/Ignored/Failed).
type Indexer struct {
	bot      *tgbot.Client
	feedChat int64 // tg_chat_id закрытого канала (источник видео ленты)
	repos    *store.Repos
}

// New — конструктор Indexer.
//
// Параметры:
//   - bot: клиент Bot API (отправка сообщений, одобрение запросов, экспорт ссылок).
//   - repos: агрегатор репозиториев store (Videos, Feed, Shops, Stats, и т.д.).
//   - feedChat: tg_chat_id закрытого канала, который индексируется.
func New(bot *tgbot.Client, repos *store.Repos, feedChat int64) *Indexer {
	return &Indexer{
		bot:      bot,
		feedChat: feedChat,
		repos:    repos,
	}
}

// HandleResult — результат обработки одного update.
type HandleResult int

const (
	Handled HandleResult = iota // обработано и записано в БД / отправлен ответ
	Ignored                     // не относится к боту (чужой чат, нет медиа)
	Failed                      // ошибка при обработке (логируется caller'ом)
)

// Handle — диспетчер обработки одного обновления u.
//
// Маршрутизация по типу события:
//
//	u.Message != nil        → handleCommand (личные сообщения боту).
//	u.ChannelPost != nil    → индексация видео (feedChat) или товара (kind=shop).
//	u.ChatJoinRequest != nil → approveJoin (только для feedChat).
//
// Возвращает одна из констант HandleResult. На ошибки индексации отвечает
// Failed; при этом сообщение в БД уже может быть записано частично.
func (ix *Indexer) Handle(ctx context.Context, u *tgbot.Update) HandleResult {
	// Личное сообщение боту → обработка команд.
	if u.Message != nil {
		return ix.handleCommand(ctx, u.Message)
	}

	// Записи из каналов: видео/фото → видео, товары из каналов-магазинов.
	if u.ChannelPost != nil {
		// Канал ленты (feedChat): каждое видео индексируем в БД.
		if u.ChannelPost.Chat.ID == ix.feedChat {
			if u.ChannelPost.Video != nil {
				if err := ix.indexVideo(ctx, u.ChannelPost); err != nil {
					log.Printf("indexer: index video failed: %v", err)
					return Failed
				}
				return Handled
			}
			// В ленту попадают только видео; текстовые посты игнорируем.
			return Ignored
		}
		// Канал-магазин (kind=shop) — индексируем товары.
		if ch, err := ix.repos.Channels.GetByTgChatID(ctx, u.ChannelPost.Chat.ID); err == nil && ch.Kind == "shop" {
			if u.ChannelPost.Video != nil || u.ChannelPost.Photo != nil {
				if err := ix.indexProduct(ctx, u.ChannelPost); err != nil {
					log.Printf("indexer: index product failed: %v", err)
					return Failed
				}
				return Handled
			}
		}
		return Ignored
	}

	// Запрос на вступление в закрытый канал → автоматическое одобрение.
	if u.ChatJoinRequest != nil && u.ChatJoinRequest.Chat.ID == ix.feedChat {
		if err := ix.approveJoin(ctx, u.ChatJoinRequest); err != nil {
			log.Printf("indexer: approve join failed: %v", err)
			return Failed
		}
		return Handled
	}
	return Ignored
}

// indexVideo — сохранение видео из канала ленты в БД.
//
// Последовательность:
//  1. EnsureByTgChatID: создаёт (или находит) канал kind="feed" в таблице channels.
//  2. Собирает store.Video: TgMsgID (для дедупликации), FileID, метаданные,
//     Title из первой строки подписи, Tags из #хэштегов.
//  3. Проверяет подпись по чёрному списку (isBlocked) → статус "banned".
//  4. Insert в БД (UNIQUE по (channel_id, tg_msg_id) запрещает дубли).
//  5. Если видео видимое — публикует в Redis-ленту со score = unix time поста.
//
// Возвращает ошибку, если Вставка в БД или публикация в ленту не удались.
func (ix *Indexer) indexVideo(ctx context.Context, m *tgbot.Message) error {
	// Гарантируем существование канала; для закрытой ленты он уже должен быть.
	channelID, err := ix.repos.Channels.EnsureByTgChatID(ctx, ix.feedChat, "feed", m.Chat.Title)
	if err != nil {
		return err
	}
	v := store.Video{
		TgMsgID:    m.MessageID,
		FileID:     m.Video.FileID,
		Caption:    m.Caption,
		DurationMs: m.Video.Duration * 1000, // Telegram отдаёт секунды, БД хранит миллисекунды
		Width:      m.Video.Width,
		Height:     m.Video.Height,
		Title:      captionToTitle(m.Caption),
		Tags:       extractTags(m.Caption),
		ChannelID:  channelID,
		PostedAt:   time.Unix(m.Date, 0),
	}

	// Фильтрация: чёрный список слов → автоматический бан (этап 5).
	if ix.isBlocked(ctx, m.Caption) {
		v.Status = "banned"
	}
	if err := ix.repos.Videos.Insert(ctx, v); err != nil {
		return err
	}
	// Публикуем в ленту Redis (score = posted_at unix).
	// При повторном insert (дубль по tg_msg_id) статус может остаться — фильтруем по "visible".
	if vid, err := ix.repos.Videos.GetByTgMsg(ctx, channelID, m.MessageID); err == nil && vid.Status == "visible" {
		return ix.repos.Feed.AddVideo(ctx, channelID, vid.ID, float64(m.Date))
	}
	return nil
}

// indexProduct — товар из канала-магазина.
//
// Выбирает file_id: у видео — m.Video.FileID, у фото — последний элемент
// photo[] (максимальное разрешение). Без медиа товар игнорируется (нет картинки —
// нечего показывать). Сохраняется со статусом "on_sale" (сразу в продаже).
func (ix *Indexer) indexProduct(ctx context.Context, m *tgbot.Message) error {
	// Ищем канал в таблице shops по tg_chat_id.
	shop, err := ix.repos.Shops.GetByTgChatID(ctx, m.Chat.ID)
	if err != nil {
		return err
	}
	var fileID string
	if m.Video != nil {
		fileID = m.Video.FileID
	} else if len(m.Photo) > 0 {
		fileID = m.Photo[len(m.Photo)-1].FileID
	}
	if fileID == "" {
		return nil // нет медиа — игнорируем
	}
	p := store.Product{
		ShopID:      shop.ID,
		TgMsgID:     m.MessageID,
		FileID:      fileID,
		Title:       captionToTitle(m.Caption),
		Description: m.Caption,
		Category:    firstTag(m.Caption),
		Status:      "on_sale",
		PostedAt:    time.Unix(m.Date, 0),
	}
	return ix.repos.Products.Insert(ctx, p)
}

// isBlocked — проверка текста по чёрному списку слов (фильтр).
// Регистронезависима (обои стороны приводятся к lower).
// Список кэшируется в Filter.List (Redis). При ошибке чтения — false (не блокируем).
func (ix *Indexer) isBlocked(ctx context.Context, text string) bool {
	words, err := ix.repos.Filter.List(ctx)
	if err != nil {
		return false
	}
	low := strings.ToLower(text)
	for _, w := range words {
		if w != "" && strings.Contains(low, strings.ToLower(w)) {
			return true
		}
	}
	return false
}

// approveJoin — автоматическое одобрение вступления пользователя в закрытый канал.
// Делегирует в ApproveJoinRequest бота; сам контракт одобрения — на стороне Telegram.
func (ix *Indexer) approveJoin(ctx context.Context, r *tgbot.ChatJoinRequest) error {
	return ix.bot.ApproveJoinRequest(r.Chat.ID, r.From.ID)
}

// captionToTitle — первая строка подписи как название.
// Если подписи нет — возвращается пустая строка (caller подставит дефолт).
func captionToTitle(caption string) string {
	if line, _, ok := strings.Cut(caption, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(caption)
}

// extractTags — #хэштеги из подписи.
// Каждое слово в подписи, начинающееся с '#' и длиннее 1 символа,
// становится тегом (без '#'). Возвращает срез без сохранения порядка.
func extractTags(caption string) []string {
	var tags []string
	for _, f := range strings.Fields(caption) {
		if strings.HasPrefix(f, "#") && len(f) > 1 {
			tags = append(tags, strings.TrimPrefix(f, "#"))
		}
	}
	return tags
}

// firstTag — первый хэштег подписи (используется как категория товаров).
// Если хэштегов нет — пустая строка.
func firstTag(s string) string {
	if tags := extractTags(s); len(tags) > 0 {
		return tags[0]
	}
	return ""
}
