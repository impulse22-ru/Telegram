package tgbot

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
)

type Client struct {
	baseURL string // напр. http://localhost:8081
	token   string
	http    *http.Client
}

func New(baseURL, token string) *Client {
	return &Client{
		baseURL: baseURL,
		token:   token,
		http:    &http.Client{Timeout: 60 * time.Second},
	}
}

// apiResponse — общая обёртка ответа Bot API.
type apiResponse[T any] struct {
	Ok          bool   `json:"ok"`
	Result      T      `json:"result"`
	Description string `json:"description"`
	ErrorCode   int    `json:"error_code"`
}

func (c *Client) call(method string, payload any, out any) error {
	var body io.Reader
	if payload != nil {
		b, err := json.Marshal(payload)
		if err != nil {
			return err
		}
		body = bytes.NewReader(b)
	}
	url := fmt.Sprintf("%s/bot%s/%s", c.baseURL, c.token, method)
	req, err := http.NewRequest(http.MethodPost, url, body)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return err
	}

	envelope := struct {
		Ok          bool            `json:"ok"`
		Result      json.RawMessage `json:"result"`
		Description string          `json:"description"`
		ErrorCode   int             `json:"error_code"`
	}{}
	if err := json.Unmarshal(data, &envelope); err != nil {
		return fmt.Errorf("bot api %s: bad response: %w", method, err)
	}
	if !envelope.Ok {
		return fmt.Errorf("bot api %s: code=%d %s", method, envelope.ErrorCode, envelope.Description)
	}
	if out != nil {
		if err := json.Unmarshal(envelope.Result, out); err != nil {
			return fmt.Errorf("bot api %s: decode result: %w", method, err)
		}
	}
	return nil
}

type GetMeUser struct {
	ID        int64  `json:"id"`
	IsBot     bool   `json:"is_bot"`
	FirstName string `json:"first_name"`
	Username  string `json:"username"`
}

func (c *Client) GetMe() (*GetMeUser, error) {
	var out GetMeUser
	if err := c.call("getMe", nil, &out); err != nil {
		return nil, err
	}
	return &out, nil
}

type GetUpdatesReq struct {
	Offset         int      `json:"offset"`
	Limit          int      `json:"limit,omitempty"`
	Timeout        int      `json:"timeout,omitempty"`
	AllowedUpdates []string `json:"allowed_updates,omitempty"`
}

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

func (c *Client) GetFile(fileID string) (*File, error) {
	var out File
	err := c.call("getFile", map[string]string{"file_id": fileID}, &out)
	return &out, err
}

// FileURL — прямой (GET) URL файла локального Bot API: /file/bot<token>/<path>.
func (c *Client) FileURL(f *File) string {
	return fmt.Sprintf("%s/file/bot%s/%s", c.baseURL, c.token, f.FilePath)
}

type ExportInviteLinkReq struct {
	ChatID int64 `json:"chat_id"`
}

func (c *Client) ExportChatInviteLink(chatID int64) (string, error) {
	var out struct {
		Link string `json:"invite_link"`
	}
	err := c.call("exportChatInviteLink", ExportInviteLinkReq{ChatID: chatID}, &out)
	return out.Link, err
}

type ApproveJoinRequestReq struct {
	ChatID int64 `json:"chat_id"`
	UserID int64 `json:"user_id"`
}

func (c *Client) ApproveJoinRequest(chatID, userID int64) error {
	return c.call("approveChatJoinRequest", ApproveJoinRequestReq{ChatID: chatID, UserID: userID}, nil)
}