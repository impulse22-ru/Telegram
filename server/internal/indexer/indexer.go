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
//   - chat_join_request → автоматическое одобрение вступления
type Indexer struct {
	bot      *tgbot.Client
	feedChat int64          // tg_chat_id закрытого канала
	channels store.Channels
	videos   store.Videos
}

func New(bot *tgbot.Client, repos *store.Repos, feedChat int64) *Indexer {
	return &Indexer{
		bot:      bot,
		feedChat: feedChat,
		channels: repos.Channels,
		videos:   repos.Videos,
	}
}

type HandleResult int

const (
	Handled  HandleResult = iota
	Ignored
	Failed
)

func (ix *Indexer) Handle(ctx context.Context, u *tgbot.Update) HandleResult {
	// Записи из ленточного канала (video/photo) → видео в БД.
	if u.ChannelPost != nil {
		if u.ChannelPost.Chat.ID == ix.feedChat && u.ChannelPost.Video != nil {
			if err := ix.indexVideo(ctx, u.ChannelPost); err != nil {
				log.Printf("indexer: index video failed: %v", err)
				return Failed
			}
			return Handled
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
	channelID, err := ix.channels.EnsureByTgChatID(ctx, ix.feedChat, "feed", m.Chat.Title)
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
		ChannelID:  channelID,
		PostedAt:   time.Unix(m.Date, 0),
	}
	return ix.videos.Insert(ctx, v)
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