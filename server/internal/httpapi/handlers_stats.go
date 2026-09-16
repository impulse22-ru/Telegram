package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tgcloud/server/internal/store"
)

// handleVideoStats — GET /v1/videos/{id}/stats. Аналитика просмотров конкретного видео
// (authMW). Возвращает совокупную статистику (likes, comments, views) и разбивку по дням;
// период days задаётся query-параметром (1..90, по умолчанию 7).
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
	likes, err := s.repos.Engagements.CountLikes(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	comments, err := s.repos.Engagements.CountComments(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	views, err := s.repos.Engagements.CountViews(r.Context(), id)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	days := 7
	// Разбивка просмотров по дням: ограничение диапазона 1..90.
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
	writeJSON(w, http.StatusOK, map[string]any{"stat": v, "by_day": byDay, "likes": likes, "comments": comments, "views": views})
}

// handleMyStats — GET /v1/stats/me. Статистика текущего пользователя (authMW):
// просмотры, лайки, комментарии и т.п. (UserStats агрегирует активность по userID).
func (s *Server) handleMyStats(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	if claims == nil {
		writeErr(w, http.StatusUnauthorized, "no auth")
		return
	}
	st, err := s.repos.Stats.UserStats(r.Context(), claims.UserID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleAdminStats — GET /v1/admin/stats. Общий дашборд для админа (authMW + adminMW).
func (s *Server) handleAdminStats(w http.ResponseWriter, r *http.Request) {
	st, err := s.repos.Stats.AdminStats(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, st)
}

// handleAdminTop — GET /v1/admin/top. Топ видео по просмотрам для админа
// (authMW + adminMW). Всегда возвращает 10 позиций.
func (s *Server) handleAdminTop(w http.ResponseWriter, r *http.Request) {
	top, err := s.repos.Stats.TopVideos(r.Context(), 10)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"top": top})
}

// handleSalesStats — GET /v1/stats/sales. Аналитика продаж текущего пользователя
// (authMW). SellerStats агрегирует выручку и объёмы по магазинам владельца.
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

// handleAdminReports — GET /v1/admin/reports?status=open. Список жалоб для модерации
// (authMW + adminMW). Фильтр по статусу (open/reviewed/dismissed), пустой — все.
func (s *Server) handleAdminReports(w http.ResponseWriter, r *http.Request) {
	status := r.URL.Query().Get("status")
	reports, err := s.repos.Engagements.Reports(r.Context(), status, 100)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"reports": reports})
}

// handleAdminReportStatus — POST /v1/admin/reports/{id}/status. Смена статуса жалобы
// (authMW + adminMW; "reviewed"|"dismissed"). Статус обязателен, иначе 400.
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

// handleAdminVideoBan — POST /v1/admin/videos/{id}/ban. Бан видео (authMW + adminMW).
// Помимо отметки в БД удаляет видео из Redis-ленты канала, чтобы оно исчезло из фида.
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

// handleAdminVideoUnban — POST /v1/admin/videos/{id}/unban. Снятие бана с видео
// (authMW + adminMW). Видео возвращается в ленту при следующей индексации.
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

// handleAdminVideoDelete — DELETE /v1/admin/videos/{id}. Мягкое удаление видео админом
// (authMW + adminMW). Удалённое видео исчезает из ленты/поиска.
func (s *Server) handleAdminVideoDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Videos.Delete(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// handleAdminCommentDelete — DELETE /v1/admin/comments/{id}. Удаление комментария админом
// (authMW + adminMW).
func (s *Server) handleAdminCommentDelete(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Engagements.DeleteComment(r.Context(), id); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"deleted": true})
}

// handleAdminShopSuspend — POST /v1/admin/shops/{id}/suspend. Приостановка магазина
// админом (authMW + adminMW). Приостановленный магазин исключается из каталога.
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

// handleFilterWordsList — GET /v1/admin/filter-words. Список запрещённых слов
// (authMW + adminMW). Слова используются фильтром комментариев.
func (s *Server) handleFilterWordsList(w http.ResponseWriter, r *http.Request) {
	words, err := s.repos.Filter.List(r.Context())
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"words": words})
}

// handleFilterWordAdd — POST /v1/admin/filter-word. Добавление запрещённого слова
// (authMW + adminMW). Пустое слово не принимается (400).
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

// handleFilterWordRemove — DELETE /v1/admin/filter-word/{word}. Удаление запрещённого
// слова (authMW + adminMW). Слово берётся из пути (URL-декодируется автоматически).
func (s *Server) handleFilterWordRemove(w http.ResponseWriter, r *http.Request) {
	word := r.PathValue("word")
	if err := s.repos.Filter.Remove(r.Context(), word); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"removed": true})
}

// handleAdminUserBan — POST /v1/admin/users/{id}/ban. Бан пользователя (authMW + adminMW).
// Edge-case: бан самого себя запрещён (400 cannot ban self).
func (s *Server) handleAdminUserBan(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	// Защита от бана самого себя — иначе админ потеряет доступ.
	if id == claims.UserID {
		writeErr(w, http.StatusBadRequest, "cannot ban self")
		return
	}
	if err := s.repos.Users.SetBanned(r.Context(), id, true); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"banned": true})
}

// handleAdminUserUnban — POST /v1/admin/users/{id}/unban. Разбан пользователя
// (authMW + adminMW).
func (s *Server) handleAdminUserUnban(w http.ResponseWriter, r *http.Request) {
	id, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	if err := s.repos.Users.SetBanned(r.Context(), id, false); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"unbanned": true})
}

// handleAdminCommentsList — GET /v1/admin/comments?video_id=&limit=. Список комментариев
// (authMW + adminMW; не удалённых). Фильтр по video_id опционален: без него — все.
// limit ограничен 200, по умолчанию 50.
func (s *Server) handleAdminCommentsList(w http.ResponseWriter, r *http.Request) {
	videoID, _ := strconv.ParseInt(r.URL.Query().Get("video_id"), 10, 64)
	limit, _ := strconv.ParseInt(r.URL.Query().Get("limit"), 10, 64)
	if limit <= 0 || limit > 200 {
		limit = 50
	}
	var out []store.Comment
	var err error
	if videoID > 0 {
		out, err = s.repos.Engagements.Comments(r.Context(), videoID, limit)
	} else {
		out, err = s.repos.Engagements.CommentsAll(r.Context(), limit)
	}
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"comments": out})
}
