package httpapi

import (
	"encoding/json"
	"errors"
	"net/http"

	"tgcloud/server/internal/auth"
)

// handleAuthRefresh — POST /v1/auth/refresh. Ротация refresh-токена.
// Принимает {"refresh_token": ...}, проверяет его в Redis-хранилище,
// отзывает старый и выдаёт новую пару: access (JWT) + refresh (opaque).
// Публичный эндпоинт (без authMW) — нужен именно при истёкшем access-токене.
func (s *Server) handleAuthRefresh(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if req.RefreshToken == "" {
		writeErr(w, http.StatusBadRequest, "refresh_token required")
		return
	}
	// Проверка + одноразовая ротация в хранилище.
	uid, newRefresh, err := s.auth.Rotate(req.RefreshToken)
	if err != nil {
		if errors.Is(err, auth.ErrNoRefresh) {
			writeErr(w, http.StatusUnauthorized, "invalid refresh token")
			return
		}
		writeErr(w, http.StatusServiceUnavailable, "refresh unavailable")
		return
	}
	user, err := s.repos.Users.Get(r.Context(), uid)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// Выдаём свежий access-токен для новой сессии.
	access, err := s.auth.Issue(user.ID, user.TgUserID, user.Role)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "token error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{
		"token":         access,
		"refresh_token": newRefresh,
		"user":          user,
	})
}

// handleAuthLogout — POST /v1/auth/logout. Отзыв refresh-токена (выход).
// Принимает {"refresh_token": ...}, удаляет сессию из Redis — после этого
// токен больше не сможет быть использован для ротации.
func (s *Server) handleAuthLogout(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RefreshToken string `json:"refresh_token"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	if req.RefreshToken != "" {
		if err := s.auth.Revoke(req.RefreshToken); err != nil {
			if errors.Is(err, auth.ErrNoSessionStore) {
				writeErr(w, http.StatusServiceUnavailable, "refresh unavailable")
				return
			}
			// Отсутствующий токен — не ошибка: цель (отзыв) и так достигнута.
		}
	}
	writeJSON(w, http.StatusOK, map[string]any{"logout": true})
}
