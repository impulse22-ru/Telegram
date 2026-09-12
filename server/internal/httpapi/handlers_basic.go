package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

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
	ch, err := s.repos.Channels.GetByTgChatID(r.Context(), s.feedChannelID)
	if err != nil {
		writeJSON(w, http.StatusOK, map[string]any{"videos": []store.Video{}})
		return
	}
	videos, err := s.repos.Videos.VisibleFrom(r.Context(), ch.ID, 0, 50)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"videos": videos})
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
	writeJSON(w, http.StatusOK, video)
}