package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tgcloud/server/internal/store"
)

// handleReviewAdd — POST /v1/product/{id}/review. Оставить или обновить отзыв на товар
// (authMW + rlMW). Rating обязателен (1..5), текст опционален.
// После добавления возвращает обновлённые avg-rating и количество отзывов.
func (s *Server) handleReviewAdd(w http.ResponseWriter, r *http.Request) {
	claims := claimsFrom(r.Context())
	productID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	var req struct {
		Rating int    `json:"rating"`
		Text   string `json:"text"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		writeErr(w, http.StatusBadRequest, "bad json")
		return
	}
	// Валидация рейтинга: допустим диапазон 1..5.
	if req.Rating < 1 || req.Rating > 5 {
		writeErr(w, http.StatusBadRequest, "rating must be 1..5")
		return
	}
	rev := store.Review{
		UserID:    claims.UserID,
		ProductID: productID,
		Rating:    req.Rating,
		Text:      req.Text,
	}
	if err := s.repos.Reviews.Add(r.Context(), rev); err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	// После добавления пересчитываем средний рейтинг — клиент обновит UI.
	avg, count, _ := s.repos.Reviews.AvgRating(r.Context(), productID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "avg": avg, "count": count})
}

// handleReviewsByProduct — GET /v1/product/{id}/reviews. Список отзывов о товаре
// (authMW, без rlMW). Возвращает массив reviews, средний рейтинг avg и количество.
func (s *Server) handleReviewsByProduct(w http.ResponseWriter, r *http.Request) {
	productID, err := strconv.ParseInt(r.PathValue("id"), 10, 64)
	if err != nil {
		writeErr(w, http.StatusBadRequest, "bad id")
		return
	}
	reviews, err := s.repos.Reviews.ByProduct(r.Context(), productID)
	if err != nil {
		writeErr(w, http.StatusInternalServerError, "db error")
		return
	}
	avg, count, _ := s.repos.Reviews.AvgRating(r.Context(), productID)
	writeJSON(w, http.StatusOK, map[string]any{
		"reviews": reviews,
		"avg":     avg,
		"count":   count,
	})
}
