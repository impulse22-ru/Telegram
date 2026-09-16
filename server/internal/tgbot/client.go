package tgbot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

// Client — обёртка над Telegram Bot API (локальный Bot API на базе tdlib).
//
// Обёрнутые методы (каждый — один API-вызов):
//   - getMe — проверка токена, получение инфо о боте.
//   - getUpdates — long polling обновлений (см. GetUpdates).
//   - getFile — метаданные файла (file_path для скачивания).
//   - exportChatInviteLink — пригласительная ссылка в закрытый канал.
//   - approveChatJoinRequest — одобрение запроса вступления.
//   - sendMessage — отправка текстового сообщения.
//   - createNewChannel — создание публичного канала.
//
// Формат запроса: POST {baseURL}/bot{token}/{method} с JSON-телом.
// Формат ответа: {"ok": bool, "result": ..., "description": ..., "error_code": int}.
type Client struct {
	baseURL string       // напр. http://localhost:8081
	token   string       // токен от BotFather / t.me/BotFather
	http    *http.Client // HTTP-клиент с таймаутом 60 сек (long polling)
}

// New — конструктор клиента Bot API.
//
// Параметры:
//   - baseURL: базовый адрес Bot API, например "http://localhost:8081" (локальный).
//   - token: токен бота.
//
// Таймаут HTTP-клиента 60 секунд нужен, чтобы переживать long polling (getUpdates с timeout).
func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// apiResponse — общая обёртка ответа Bot API.
type apiResponse[T any] struct {
	Ok          bool   `json:"ok"`          // успех вызова
	Result      T      `json:"result"`      // полезная нагрузка (зависит от метода)
	Description string `json:"description"` // человекочитаемое описание ошибки
	ErrorCode   int    `json:"error_code"`  // код ошибки Telegram
}

// call — низкоуровневый HTTP-метод для вызова Bot API.
//
// Делает POST {baseURL}/bot{token}/{method} с JSON-телом payload.
// Ответ парсится в два этапа:
//  1. Конверт ({ok, result, description, error_code}) — для единой проверки ошибок.
//  2. result (RawMessage) — в выходную структуру out (если out != nil).
//
// Ошибки: сетевые, код != 200, корпус = {"ok": false}, ошибка декодирования.
// Пулл: httptest для локальной разработки тоже подходит (без реального Telegram).
func (c *Client) call(method string, payload any, out any) error {
	var body io.Reader
	// Кодируем payload в JSON, если он задан.
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}

	// URL вида: http://localhost:8081/bot123456:AA.../getMe
	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	// Выполняем запрос и читаем тело ответа.
	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	// Разбираем общий конверт ответа.
	envelope := struct {
		Ok          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}{}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("bot api %s: bad response: %w", method, err)
	}

	// Telegram возвращает {"ok": false, "description": "...", "error_code": N} при ошибке API.
	if !envelope.Ok {
		return fmt.Errorf("bot api %s: code=%d %s", method, envelope.ErrorCode, envelope.Description)
	}

	// Декодируем result в целевую структуру.
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("bot api %s: decode result: %w", method, err)
		}
	}
	return nil
}

// GetMeUser — данные текущего бота из метода getMe.
type GetMeUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

// GetMe — вызывает getMe, возвращает профиль бота.
// Используется для проверки живости Bot API и получения имени бота.
// Возвращает *GetMeUser или ошибку (невалидный токен, недоступный API).
func (c *Client) GetMe() (*GetMeUser, error) {
	var out GetMeUser
	if err := c.call("getMe", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

// GetUpdatesReq — параметры long polling метода getUpdates.
//   - Offset: номер первого обрабатываемого update (N+1 после последнего обработанного).
//   - Limit: максимум возвращаемых updates (1..100).
//   - Timeout: сколько секунд держать соединение, если новых апдейтов нет (обычно 30).
//   - AllowedUpdates: список типов апдейтов; пустой — все типы.
type GetUpdatesReq struct {
	Offset         int      `json:"offset"`
	Limit          int      `json:"limit,omitempty"`
	Timeout        int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

// GetUpdates — long polling обновлений.
//
// Безусловно переопределяет AllowedUpdates на ["message", "channel_post", "chat_join_request"],
// т.к. именно эти типы обрабатываются indexer'ом. Это сокращает трафик и исключает лишние апдейты
// (edited_message, callback_query и т.д.).
func (c *Client) GetUpdates(req GetUpdatesReq) ([]Update, error) {
	var out []Update
	req.AllowedUpdates = []string{"message", "channel_post", "chat_join_request"}
	err := c.call("getUpdates", req, &out)
	return out, err
}

// File — метаданные медиа из getFile.
type File struct {
	FileID   string `json:"file_id"`
	FileSize int64  `json:"file_size"`
	FilePath string `json:"file_path"`
}

// GetFile — получает метаданные файла по file_id.
// Result служит для построения download URL (см. FileURL).
// Возвращает *File с non-empty FilePath в случае успеха.
func (c *Client) GetFile(fileID string) (*File, error) {
	var out File
	err := c.call("getFile", map[string]string{"file_id": fileID}, &out)
	return &out, err
}

// FileURL — прямой (GET) URL файла локального Bot API: /file/bot<token>/<path>.
//
// Используется строго с локальным Bot API (python-telegram-bot / tdlib),
// т.к. облачный Bot API (api.telegram.org) не раздаёт файлы с этой схемой.
// По этому URL relay скачивает видео для кэша.
func (c *Client) FileURL(f *File) string {
	return fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, c.token, f.FilePath)
}

// ExportInviteLinkReq — параметры метода exportChatInviteLink.
type ExportInviteLinkReq struct {
	ChatID int64 `json:"chat_id"`
}

// ExportChatInviteLink — создаёт разовую пригласительную ссылку в чат.
// Используется командой /start для приглашения пользователей в закрытый канал ленты.
// Возвращает строку ссылки (без префикса) или ошибку.
func (c *Client) ExportChatInviteLink(chatID int64) (string, error) {
	var out struct {
		Link string `json:"invite_link"`
	}
	err := c.call("exportChatInviteLink", ExportInviteLinkReq{ChatID: chatID}, &out)
	return out.Link, err
}

// ApproveJoinRequestReq — параметры одобрения запроса на вступление.
type ApproveJoinRequestReq struct {
	ChatID int64 `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

// ApproveJoinRequest — одобряет запрос пользователя на вступление в закрытый чат.
// Вызывается автоматически при chat_join_request (см. Indexer.approveJoin).
func (c *Client) ApproveJoinRequest(chatID, userID int64) error {
	return c.call("approveChatJoinRequest", ApproveJoinRequestReq{ChatID: chatID, UserID: userID}, nil)
}

// SendMessageReq — параметры метода sendMessage.
type SendMessageReq struct {
	ChatID int64  `json:"chat_id"`
	Text   string `json:"text"`
}

// SendMessage — отправляет текст в чат (бот создаётся через sendMessage).
// Используется для ответов на команды в личке (/start, !search и т.д.).
func (c *Client) SendMessage(chatID int64, text string) error {
	return c.call("sendMessage", SendMessageReq{ChatID: chatID, Text: text}, nil)
}

// CreateChannel — создаёт публичный канал (бот становится владельцем) и
// возвращает его tg_chat_id.
//
// В методе используется createNewChannel — аналог @ChannelBot / обсуждаемой
// автоматизации. Канал создаётся с заголовком title и необязательным
// description (добавляется только если не пустой).
func (c *Client) CreateChannel(title, description string) (int64, error) {
	var out struct {
		Chat struct {
			ID int64 `json:"id"` // id созданного канала
		} `json:"chat"`
	}
	payload := map[string]string{"title": title}
	if description != "" {
		payload["description"] = description
	}
	if err := c.call("createNewChannel", payload, &out); err != nil {
		return 0, err
	}
	return out.Chat.ID, nil
}
