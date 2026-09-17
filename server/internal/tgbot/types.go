package tgbot

// Update — одно событие из getUpdates (используем подмножество полей).
//
// Поля-указатели (Message, ChannelPost, ChatJoinRequest) взаимно исключают друг
// друга: в одном update заполнено ровно одно из них. Тип события определяет
// обработку в indexer:
//   - Message: личное сообщение боту → команды.
//   - ChannelPost: запись в канале → индексация видео/товаров.
//   - ChatJoinRequest: запрос на вступление → автодобавление.
type Update struct {
	UpdateID        int64            `json:"update_id"`
	Message         *Message         `json:"message"`
	ChannelPost     *Message         `json:"channel_post"`
	ChatJoinRequest *ChatJoinRequest `json:"chat_join_request"`
}

// Message — сообщение в Telegram (личное или запись канала).
// Video и Photo заполняются только для медиасообщений.
// Caption — подпись к медиа (для видео из ленты — источник названия и тегов).
type Message struct {
	MessageID int64       `json:"message_id"`
	Chat      Chat        `json:"chat"`    // чат, в котором опубликовано сообщение
	From      User        `json:"from"`    // автор сообщения
	Date      int64       `json:"date"`    // unix-время публикации
	Text      string      `json:"text"`    // текстовое содержимое (личные сообщения)
	Caption   string      `json:"caption"` // подпись к медиа (видео/фото)
	Video     *Video      `json:"video"`
	Photo     []PhotoSize `json:"photo"`
}

// PhotoSize — размер фото из массива photo[]. Telegram присылает несколько
// вариантов (thumbnail → оригинал); берём последний — максимальное разрешение.
type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

// Chat — чат (канал/группа/личный диалог).
// ID — int64, уникальный в масштабах Telegram. Для каналов Type == "channel".
type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

// Video — видеосообщение из канала.
// Duration указан в секундах — при сохранении в БД переводится в миллисекунды.
// FileID — идентификатор для метода getFile (скачивание через relay).
type Video struct {
	FileID       string `json:"file_id"`
	FileUniqueID string `json:"file_unique_id"`
	Width        int    `json:"width"`
	Height       int    `json:"height"`
	Duration     int    `json:"duration"` // секунды
	MimeType     string `json:"mime_type"`
	FileSize     int64  `json:"file_size"`
}

// ChatJoinRequest — запрос пользователя на вступление в закрытый чат.
// Приходит обновлением chat_join_request; для автоматического одобрения
// вызывается approveJoinRequest с Chat.ID и From.ID.
type ChatJoinRequest struct {
	Chat Chat  `json:"chat"`
	From User  `json:"from"`
	Date int64 `json:"date"`
}

// User — пользователь Telegram.
// ID — Telegram user id (используется в авторизации как TgUserID).
// Username — публичный @username (без символа @).
type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}
