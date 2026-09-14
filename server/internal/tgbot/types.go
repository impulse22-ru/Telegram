package tgbot

// Update — одно событие из getUpdates (используем подмножество полей).
type Update struct {
	UpdateID        int64             `json:"update_id"`
	Message         *Message          `json:"message"`
	ChannelPost     *Message          `json:"channel_post"`
	ChatJoinRequest *ChatJoinRequest  `json:"chat_join_request"`
}

type Message struct {
	MessageID int64  `json:"message_id"`
	Chat      Chat   `json:"chat"`
	From      User   `json:"from"`
	Date      int64  `json:"date"`
	Text      string `json:"text"`
	Caption   string `json:"caption"`
	Video     *Video `json:"video"`
	Photo     []PhotoSize `json:"photo"`
}

type PhotoSize struct {
	FileID   string `json:"file_id"`
	Width    int    `json:"width"`
	Height   int    `json:"height"`
	FileSize int64  `json:"file_size"`
}

type Chat struct {
	ID       int64  `json:"id"`
	Type     string `json:"type"`
	Title    string `json:"title"`
	Username string `json:"username"`
}

type Video struct {
	FileID     string `json:"file_id"`
	Width      int    `json:"width"`
	Height     int    `json:"height"`
	Duration   int    `json:"duration"` // секунды
	MimeType   string `json:"mime_type"`
	FileSize   int64  `json:"file_size"`
}

type ChatJoinRequest struct {
	Chat  Chat  `json:"chat"`
	From  User  `json:"from"`
	Date  int64 `json:"date"`
}

type User struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	LastName  string `json:"last_name"`
	Username  string `json:"username"`
}