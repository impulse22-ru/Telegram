package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// handleLike — POST /v1/videos/{id}/like. Поставить лайк видео (authMW + rlMW).
// Использует userAndVideo для извлечения userID и videoID из контекста/пути.
func (s *Server) handleLike(w http.ResponseWriter, r *http.Request) {
	uid, vid, ok := s.userAndVideo(w, r)
	if !ok {
		return
	}
	if err := s.repos.Engagements.Like(r.Context(), uid, vid); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": true})
}

// handleUnlike — POST /v1/videos/{id}/unlike. Снять лайк с видео (authMW + rlMW).
func (s *Server) handleUnlike(w http.ResponseWriter, r *http.Request) {
	uid, vid, ok := s.userAndVideo(w, r)
	if !ok {
		return
	}
	if err := s.repos.Engagements.Unlike(r.Context(), uid, vid); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"liked": false})
}

// handleCommentsList — GET /v1/videos/{id}/comments. Список комментариев к видео
// (authMW, без rlMW — чтение не лимитируется).
func (s *Server) handleCommentsList(w http.ResponseWriter, r *http.Request) {
	_, vid, ok := s.userAndVideo(w, r)
	if !ok {
		return
	}
	comments, err := s.repos.Engagements.Comments(r.Context(), vid, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": comments})
}

// handleComment — POST /v1/videos/{id}/comment. Добавить комментарий к видео
// (authMW + rlMW). Поддерживает вложенные комментарии через parent_id.
func (s *Server) handleComment(w http.ResponseWriter, r *http.Request) {
	uid, vid, ok := s.userAndVideo(w, r)
	if !ok {
		return
	}
	var req struct {
		Text     string `json:"text"`
		ParentID int64  `json:"parent_id"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Text == "" {
		writeErr(w, http.StatusBadRequest, "text required")
		return
	}
	id, err := s.repos.Engagements.Comment(r.Context(), uid, vid, req.Text, req.ParentID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comment_id": id})
}

// handleReport — POST /v1/videos/{id}/report. Пожаловаться на видео (authMW + rlMW).
// Причина опциональна — если не указана, отправляется пустая строка.
func (s *Server) handleReport(w http.ResponseWriter, r *http.Request) {
	uid, vid, ok := s.userAndVideo(w, r)
	if !ok {
		return
	}
	var req struct {
		Reason string `json:"reason"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.repos.Engagements.Report(r.Context(), uid, vid, req.Reason); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"flagged": true})
}

// handleView — POST /v1/videos/{id}/view. Зафиксировать просмотр видео (authMW).
// watch_seconds — сколько секунд пользователь реально смотрел (для скоринга).
func (s *Server) handleView(w http.ResponseWriter, r *http.Request) {
	uid, vid, ok := s.userAndVideo(w, r)
	if !ok {
		return
	}
	var req struct {
		WatchSeconds int `json:"watch_seconds"`
	}
	_ = json.NewDecoder(r.Body).Decode(&req)
	if err := s.repos.Engagements.RecordView(r.Context(), uid, vid, req.WatchSeconds); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"viewed": true})
}

// userAndVideo — вспомогательная функция: извлекает userID из claims в контексте
// и videoID из пути запроса. Возвращает (userID, videoID, ok).
// Используется всеми хендлерами engagement-эндпоинтов.
func (s *Server) userAndVideo(w http.ResponseWriter, r *http.Request) (int64, int64, bool) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "no claims")
		return 0, 0, false
	}
	vid, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return 0, 0, false
	}
	userID := claims.UserID
	return userID, vid, true
}
