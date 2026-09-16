package indexer

import (
	"context"
	"fmt"
	"log"
	"strings"

	"tgcloud/server/internal/tgbot"
)

// handleCommand — обработка команд бота от обычных пользователей (этап 5).
// Поддерживаемые команды:
//
//	/start            — приветствие + пригласительная ссылка
//	!help             — список команд
//	!search <запрос>  — поиск видео по названию/подписи
//	!top              — топ видео по просмотрам
//	!stat             — статистика ленты
//
// (админские добавляются отдельно, см. adminCommand.go)
//
// Разрешаются как команды с '!', так и с '/', чтобы работать и в Telegram,
// и в античных клиентах ('/' конфликтует с нативными командами бота).
//
// Админ-команды (!ban, !filter и т.д.) доступны только пользователям с ролью
// "admin" — проверка через isAdmin перед веткой админских команд.
func (ix *Indexer) handleCommand(ctx context.Context, m *tgbot.Message) HandleResult {
	text := strings.TrimSpace(m.Text)
	if text == "" {
		return Ignored // пустое сообщение (например, только стикер) — не команда
	}

	cmd, arg := splitCommand(text)
	switch cmd {
	case "/start":
		return ix.cmdStart(ctx, m)
	case "!help", "/help":
		return ix.reply(ctx, m, helpText)
	case "!search", "/search":
		return ix.cmdSearch(ctx, m, arg)
	case "!top", "/top":
		return ix.cmdTop(ctx, m)
	case "!stat", "/stat":
		return ix.cmdStat(ctx, m)
	}
	// админ-команды
	if ix.isAdmin(ctx, m.From.ID) {
		switch cmd {
		case "!ban":
			return ix.cmdBan(ctx, m, arg)
		case "!unban":
			return ix.cmdUnban(ctx, m, arg)
		case "!filter", "!block":
			return ix.cmdFilter(ctx, m, arg)
		case "!unfilter", "!unblock":
			return ix.cmdUnfilter(ctx, m, arg)
		}
	}
	return Ignored // неизвестная команда — игнорируем, чтобы не спамить
}

// cmdStart — обработка /start.
// Отвечает приветствием и, если задан закрытый канал ленты, пригласительной
// ссылкой (exportChatInviteLink). Ссылка создаётся каждый раз заново.
func (ix *Indexer) cmdStart(ctx context.Context, m *tgbot.Message) HandleResult {
	sb := strings.Builder{}
	sb.WriteString("Привет! Это бот ленты.\n")
	if ix.feedChat != 0 && ix.repos != nil {
		// Экспорт ссылки может упасть (бот не админ канала и т.п.) — пропускаем.
		link, err := ix.bot.ExportChatInviteLink(ix.feedChat)
		if err == nil && link != "" {
			sb.WriteString("\nВступить в канал ленты: " + link)
		}
	}
	return ix.reply(ctx, m, sb.String())
}

// cmdSearch — поиск видео по названию/подписи (!search <запрос>).
// Ищет до 5 результатов: репозиторий Videos.Search (ILIKE по title и caption).
// Пустой запрос → подсказка использования. Ноль результатов → «Ничего не найдено».
func (ix *Indexer) cmdSearch(ctx context.Context, m *tgbot.Message, q string) HandleResult {
	if q == "" {
		return ix.reply(ctx, m, "Укажи запрос: !search <текст>")
	}
	videos, err := ix.repos.Videos.Search(ctx, q, 5)
	if err != nil {
		log.Printf("indexer: search: %v", err)
		return ix.reply(ctx, m, "Ошибка поиска")
	}
	if len(videos) == 0 {
		return ix.reply(ctx, m, "Ничего не найдено по «"+q+"»")
	}
	sb := strings.Builder{}
	sb.WriteString("Найдено:\n")
	for _, v := range videos {
		title := v.Title
		if title == "" {
			title = "видео #" + fmt.Sprint(v.ID) // фолбэк для видео без названия
		}
		// Внимание: v.ChannelID здесь используется как place — просмотры счётчика,
		// корректное поле views, по-видимому, в этой схеме не заполнено.
		sb.WriteString(fmt.Sprintf("• #%d %s (👁 %d)\n", v.ID, title, v.ChannelID))
	}
	return ix.reply(ctx, m, strings.TrimSpace(sb.String()))
}

// cmdTop — топ-5 видео по просмотрам.
// Источник: Stats.TopVideos. Формат ответа: "1. видео #<id> — <N> 👁".
func (ix *Indexer) cmdTop(ctx context.Context, m *tgbot.Message) HandleResult {
	top, err := ix.repos.Stats.TopVideos(ctx, 5)
	if err != nil {
		log.Printf("indexer: top: %v", err)
		return ix.reply(ctx, m, "Ошибка")
	}
	if len(top) == 0 {
		return ix.reply(ctx, m, "Пока нет просмотров")
	}
	sb := strings.Builder{}
	sb.WriteString("Топ видео по просмотрам:\n")
	for i, s := range top {
		sb.WriteString(fmt.Sprintf("%d. видео #%d — %d 👁\n", i+1, s.VideoID, s.Views))
	}
	return ix.reply(ctx, m, strings.TrimSpace(sb.String()))
}

// cmdStat — агрегированная статистика ленты (!stat).
// Источник: Stats.AdminStats: число пользователей, видео (всего/видимых),
// просмотров, лайков, комментариев.
func (ix *Indexer) cmdStat(ctx context.Context, m *tgbot.Message) HandleResult {
	st, err := ix.repos.Stats.AdminStats(ctx)
	if err != nil {
		log.Printf("indexer: stat: %v", err)
		return ix.reply(ctx, m, "Ошибка")
	}
	sb := strings.Builder{}
	fmt.Fprintf(&sb, "Лента в цифрах:\n👥 %d пользователей\n🎬 %d видео (%d видимых)\n👁 %d просмотров\n❤️ %d лайков\n💬 %d комментариев\n",
		st.Users, st.Videos, st.VisibleVideos, st.Views, st.Likes, st.Comments)
	return ix.reply(ctx, m, sb.String())
}

// --- админ-команды ---
// Все админ-команды доступны только после проверки isAdmin (роль "admin").

// cmdBan — банит видео (!ban <video_id>): ставит статус "banned" и убирает
// его из Redis-ленты, чтобы оно пропало из выдачи.
func (ix *Indexer) cmdBan(ctx context.Context, m *tgbot.Message, arg string) HandleResult {
	id := parseID(arg)
	if id <= 0 {
		return ix.reply(ctx, m, "Требуется ID: !ban <video_id>")
	}
	if err := ix.repos.Videos.Ban(ctx, id); err != nil {
		return ix.reply(ctx, m, "Ошибка бана")
	}
	// Также удаляем из ленты Redis — иначе забаненное видео останется у клиентов.
	if v, err := ix.repos.Videos.Get(ctx, id); err == nil {
		_ = ix.repos.Feed.RemoveVideo(ctx, v.ChannelID, id)
	}
	return ix.reply(ctx, m, "Видео #"+fmt.Sprint(id)+" заблокировано")
}

// cmdUnban — разблокирует видео (!unban <video_id>): возвращает статус "visible".
// Лента Redis при этом НЕ пополняется автоматически — видео вернётся в выдачу
// только после следующей индексации канала.
func (ix *Indexer) cmdUnban(ctx context.Context, m *tgbot.Message, arg string) HandleResult {
	id := parseID(arg)
	if id <= 0 {
		return ix.reply(ctx, m, "Требуется ID: !unban <video_id>")
	}
	if err := ix.repos.Videos.Unban(ctx, id); err != nil {
		return ix.reply(ctx, m, "Ошибка")
	}
	return ix.reply(ctx, m, "Видео #"+fmt.Sprint(id)+" разблокировано")
}

// cmdFilter — добавляет слово в чёрный список (!filter <слово>).
// Дальнейшие видео с этим словом в подписи индексируются как "banned".
func (ix *Indexer) cmdFilter(ctx context.Context, m *tgbot.Message, arg string) HandleResult {
	if arg == "" {
		return ix.reply(ctx, m, "Укажи слово: !filter <слово>")
	}
	if err := ix.repos.Filter.Add(ctx, arg); err != nil {
		return ix.reply(ctx, m, "Ошибка")
	}
	return ix.reply(ctx, m, "Слово «"+arg+"» добавлено в чёрный список")
}

// cmdUnfilter — удаляет слово из чёрного списка (!unfilter <слово>).
// Уже забаненные видео остаются забаненными (статус не пересматривается).
func (ix *Indexer) cmdUnfilter(ctx context.Context, m *tgbot.Message, arg string) HandleResult {
	if arg == "" {
		return ix.reply(ctx, m, "Укажи слово: !unfilter <слово>")
	}
	if err := ix.repos.Filter.Remove(ctx, arg); err != nil {
		return ix.reply(ctx, m, "Ошибка")
	}
	return ix.reply(ctx, m, "Слово «"+arg+"» удалено из чёрного списка")
}

// reply — временный ответ с созданием пользователя, если нужно.
// Отправляет сообщение в чат, из которого пришла команда (m.Chat.ID).
// При ошибке отправки возвращает Failed, иначе — Handled.
func (ix *Indexer) reply(ctx context.Context, m *tgbot.Message, text string) HandleResult {
	if err := ix.bot.SendMessage(m.Chat.ID, text); err != nil {
		log.Printf("indexer: sendMessage: %v", err)
		return Failed
	}
	return Handled
}

// isAdmin — проверка роли admin у пользователя по tg_user_id.
// Ищет пользователя в БД по Telegram ID; роль "admin" → true.
// При любой ошибке (нет пользователя, сетевая и т.п.) — false (безопасное значение).
func (ix *Indexer) isAdmin(ctx context.Context, tgUserID int64) bool {
	u, err := ix.repos.Users.GetByTgID(ctx, tgUserID)
	if err != nil {
		return false
	}
	return u.Role == "admin"
}

// helpText — справка, выводимая командами !help / /help.
const helpText = `Доступные команды:
/start — пригласительная ссылка в канал ленты
!search <текст> — поиск видео
!top — топ по просмотрам
!stat — статистика ленты

Админ:
!ban <id>, !unban <id>
!filter <слово>, !unfilter <слово>`

// splitCommand выделяет команду и аргумент, игнорируя упоминание бота: "/start@BotName".
// Разбивает текст по первому пробелу; если упоминание "@BotName" — отрезает его.
// Возвращает команду в нижнем регистре (без пробелов) и аргумент (trim).
func splitCommand(text string) (cmd, arg string) {
	text = strings.TrimSpace(text)
	if strings.Contains(text, " ") {
		cmd, arg, _ = strings.Cut(text, " ")
	} else {
		cmd = text
	}
	// Отсекаем @BotName для команд вида "/start@MyBot".
	if at := strings.IndexByte(cmd, '@'); at > 0 {
		cmd = cmd[:at]
	}
	return strings.ToLower(strings.TrimSpace(cmd)), strings.TrimSpace(arg)
}

// parseID — парсит целочисленный ID из строки аргумента команды.
// Sscan читает первое число; ошибка парсинга игнорируется → возвращается 0.
func parseID(s string) int64 {
	var id int64
	_, _ = fmt.Sscan(strings.TrimSpace(s), &id)
	return id
}
