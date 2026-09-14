package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"
)

// GET /v1/videos/{id}/stats — аналитика просмотров конкретного видео.
func (s *Server) handleVideoStats(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	v, err := s.repos.Stats.VideoStat(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusNotFound, "not found")
		return
	}
	days := 7
	if sd := r.URL.Query().Get("days"); sd != "" {
		if n, err := strconv.Atoi(sd); err == nil && n > 0 && n <= 90 {
			days = n
		}
	}
	byDay, err := s.repos.Stats.VideoViewsDay(r.Context(), id, days)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"stat": v, "by_day": byDay})
}

// GET /v1/stats/me — статистика моих просмотров/лайков.
func (s *Server) handleMyStats(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{"status": "ok"})
}

// GET /v1/admin/stats — общий дашборд.
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.repos.Stats.AdminStats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// GET /v1/admin/top — топ видео по просмотрам.
func (s *Server) handleAdminTop(w http.ResponseWriter, r *http.Request) {
	top, err := s.repos.Stats.TopVideos(r.Context(), 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"top": top})
}

// GET /v1/stats/sales — аналитика продаж текущего пользователя (продавца).
func (s *Server) handleSalesStats(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	st, err := s.repos.Stats.SellerStats(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// --- Модерация (этап 7) ---

// GET /v1/admin/reports?status=open
func (s *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	reports, err := s.repos.Engagements.Reports(r.Context(), status, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

// POST /v1/admin/reports/{id}/status {"status":"reviewed"|"dismissed"}
func (s *Server) handleAdminReportStatus(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Status string `json:"status"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Status == "" {
		writeErr(w, http.StatusBadRequest, "status required")
		return
	}
	if err := s.repos.Engagements.ReportResolve(r.Context(), id, req.Status); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"updated": true})
}

// POST /v1/admin/videos/{id}/ban  (и удаление из ленты)
func (s *Server) handleAdminVideoBan(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Videos.Ban(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// удалить из Redis-ленты канала
	if v, err := s.repos.Videos.Get(r.Context(), id); err == nil {
		_ = s.repos.Feed.RemoveVideo(r.Context(), v.ChannelID, id)
	}
	writeJSON(w, http.StatusOK, map[string]any{"banned": true})
}

// POST /v1/admin/videos/{id}/unban
func (s *Server) handleAdminVideoUnban(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Videos.Unban(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unbanned": true})
}

// POST /v1/admin/shops/{id}/suspend
func (s *Server) handleAdminShopSuspend(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Shops.Suspend(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"suspended": true})
}

// --- Фильтр-слова (этап 5) ---

// GET /v1/admin/filter-words
func (s *Server) handleFilterWordsList(w http.ResponseWriter, r *http.Request) {
	words, err := s.repos.Filter.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"words": words})
}

// POST /v1/admin/filter-word {"word":"..."}
func (s *Server) handleFilterWordAdd(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Word string `json:"word"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil || req.Word == "" {
		writeErr(w, http.StatusBadRequest, "word required")
		return
	}
	if err := s.repos.Filter.Add(r.Context(), req.Word); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"added": true})
}

// DELETE /v1/admin/filter-word/{word}
func (s *Server) handleFilterWordRemove(w http.ResponseWriter, r *http.Request) {
	word := r.PathValue("word")
	if err := s.repos.Filter.Remove(r.Context(), word); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}