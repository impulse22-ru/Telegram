package httpapi

import (
	"encoding/json"
	"net/http"
	"strconv"

	"tgcloud/server/internal/store"
)

// POST /v1/product/{id}/review — оставить/обновить отзыв.
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
	avg, count, _ := s.repos.Reviews.AvgRating(r.Context(), productID)
	writeJSON(w, http.StatusOK, map[string]any{"ok": true, "avg": avg, "count": count})
}

// GET /v1/product/{id}/reviews — отзывы о товаре.
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
