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
type Indexer struct {
	bot      *tgbot.Client
	feedChat int64 // tg_chat_id закрытого канала
	repos    *store.Repos
}

func New(bot *tgbot.Client, repos *store.Repos, feedChat int64) *Indexer {
	return &Indexer{
		bot:      bot,
		feedChat: feedChat,
		repos:    repos,
	}
}

type HandleResult int

const (
	Handled  HandleResult = iota
	Ignored
	Failed
)

func (ix *Indexer) Handle(ctx context.Context, u *tgbot.Update) HandleResult {
	// Личное сообщение боту → обработка команд.
	if u.Message != nil {
		return ix.handleCommand(ctx, u.Message)
	}

	// Записи из каналов: видео/фото → видео, товары из каналов-магазинов.
	if u.ChannelPost != nil {
		if u.ChannelPost.Chat.ID == ix.feedChat {
			if u.ChannelPost.Video != nil {
				if err := ix.indexVideo(ctx, u.ChannelPost); err != nil {
					log.Printf("indexer: index video failed: %v", err)
					return Failed
				}
				return Handled
			}
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

func (ix *Indexer) indexVideo(ctx context.Context, m *tgbot.Message) error {
	channelID, err := ix.repos.Channels.EnsureByTgChatID(ctx, ix.feedChat, "feed", m.Chat.Title)
	if err != nil {
		return err
	}
	v := store.Video{
		TgMsgID:    m.MessageID,
		FileID:     m.Video.FileID,
		Caption:    m.Caption,
		DurationMs: m.Video.Duration * 1000,
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
	if vid, err := ix.repos.Videos.GetByTgMsg(ctx, channelID, m.MessageID); err == nil && vid.Status == "visible" {
		return ix.repos.Feed.AddVideo(ctx, channelID, vid.ID, float64(m.Date))
	}
	return nil
}

// indexProduct — товар из канала-магазина.
func (ix *Indexer) indexProduct(ctx context.Context, m *tgbot.Message) error {
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
		ShopID:     shop.ID,
		TgMsgID:    m.MessageID,
		FileID:     fileID,
		Title:      captionToTitle(m.Caption),
		Description: m.Caption,
		Category:   firstTag(m.Caption),
		Status:     "on_sale",
		PostedAt:   time.Unix(m.Date, 0),
	}
	return ix.repos.Products.Insert(ctx, p)
}

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

func (ix *Indexer) approveJoin(ctx context.Context, r *tgbot.ChatJoinRequest) error {
	return ix.bot.ApproveJoinRequest(r.Chat.ID, r.From.ID)
}

// captionToTitle — первая строка подписи как название.
func captionToTitle(caption string) string {
	if line, _, ok := strings.Cut(caption, "\n"); ok {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(caption)
}

// extractTags — #хэштеги из подписи.
func extractTags(caption string) []string {
	var tags []string
	for _, f := range strings.Fields(caption) {
		if strings.HasPrefix(f, "#") && len(f) > 1 {
			tags = append(tags, strings.TrimPrefix(f, "#"))
		}
	}
	return tags
}

func firstTag(s string) string {
	if tags := extractTags(s); len(tags) > 0 {
		return tags[0]
	}
	return ""
}