package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
	"strings"

	"tgcloud/server/internal/store"
)

func (s *Server) handleHealth(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

func (s *Server) handleAuthTelegram(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TgUserID int64  `json:"tg_user_id"`
		Phone    string `json:"phone"`
		Name     string `json:"name"`
		AuthHash string `json:"auth_hash"` // подпись с клиента (доп. защита)
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if req.TgUserID == 0 {
		writeErr(w, http.StatusBadRequest, "tg_user_id required")
		return
	}

	ctx := r.Context()
	uid, err := s.repos.Users.Upsert(ctx, store.User{
		TgUserID: req.TgUserID,
		Phone:    req.Phone,
		Name:     req.Name,
	})
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	user, err := s.repos.Users.Get(ctx, uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	token, err := s.auth.Issue(user.ID, user.TgUserID, user.Role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"token": token, "user": user})
}

func (s *Server) handleFeed(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	ch, err := s.repos.Channels.GetByTgChatID(ctx, s.feedChannelID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []store.Video{}, "has_more": false})
		return
	}
	offset, _ := strconv.ParseInt(r.URL.Query().Get("offset"), 10, 64)
	limit := int64(20)
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.ParseInt(v, 10, 64); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}

	// Скоринговая пагинированная лента; фолбэк на время, если скоринг пуст.
	ids, err := s.repos.Feed.ScoredFeed(ctx, ch.ID, offset, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "feed error")
		return
	}
	if len(ids) == 0 {
		ids, err = s.repos.Feed.Top(ctx, ch.ID, limit)
		if err != nil {
			writeErr(w, http.StatusInternalServerError, "feed error")
			return
		}
	}
	videos := make([]store.Video, 0, len(ids))
	for _, id := range ids {
		v, err := s.repos.Videos.Get(ctx, id)
		if err != nil {
			continue
		}
		videos = append(videos, *v)
	}
	hasMore := len(videos) == int(limit)
	writeJSON(w, http.StatusOK, map[string]any{"videos": videos, "has_more": hasMore, "offset": offset + int64(len(videos))})
}

func (s *Server) handleVideoGet(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	video, err := s.repos.Videos.Get(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	likes, err := s.repos.Engagements.CountLikes(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	views, err := s.repos.Engagements.CountViews(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	claims := claimsFrom(r.Context())
	liked := false
	if claims != nil {
		liked, _ = s.repos.Engagements.IsLiked(r.Context(), claims.UserID, id)
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"video": video,
		"likes": likes,
		"views": views,
		"liked": liked,
	})
}

// GET /v1/search?q=... — поиск видео по title/caption (аналог !search).
func (s *Server) handleSearch(w http.ResponseWriter, r *http.Request) {
	q := strings.TrimSpace(r.URL.Query().Get("q"))
	if q == "" {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []store.Video{}, "query": ""})
		return
	}
	limit := int64(20)
	if v := r.URL.Query().Get("limit"); v != "" {
		if l, err := strconv.ParseInt(v, 10, 64); err == nil && l > 0 && l <= 50 {
			limit = l
		}
	}
	videos, err := s.repos.Videos.Search(r.Context(), q, limit)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	if videos == nil {
		videos = []store.Video{}
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": videos, "query": q})
}