package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"tgcloud/server/internal/auth"
	"tgcloud/server/internal/store"
)

type Server struct {
	repos         *store.Repos
	auth          *auth.Manager
	feedChannelID int64
}

func New(repos *store.Repos, am *auth.Manager, feedChannelID int64) *Server {
	return &Server{repos: repos, auth: am, feedChannelID: feedChannelID}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("POST /v1/auth/telegram", s.handleAuthTelegram)

	mux.HandleFunc("GET /v1/feed", s.authMW(s.handleFeed))
	mux.HandleFunc("GET /v1/videos/{id}", s.authMW(s.handleVideoGet))

	mux.HandleFunc("POST /v1/shop", s.authMW(s.handleShopCreate))
	mux.HandleFunc("GET /v1/shops", s.authMW(s.handleShopsList))
	mux.HandleFunc("POST /v1/product", s.authMW(s.handleProductCreate))
	mux.HandleFunc("POST /v1/order", s.authMW(s.handleOrderCreate))
	mux.HandleFunc("GET /v1/order/{id}", s.authMW(s.handleOrderGet))
	mux.HandleFunc("POST /v1/order/{id}/confirm", s.authMW(s.handleOrderConfirm))
	mux.HandleFunc("GET /v1/orders/seller", s.authMW(s.handleOrdersSeller))

	return requestLog(mux)
}

// --- helpers ---

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

type ctxKey int

const claimsKey ctxKey = 0

func (s *Server) authMW(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		token := r.Header.Get("Authorization")
		const prefix = "Bearer "
		if len(token) < len(prefix) || token[:len(prefix)] != prefix {
			writeErr(w, http.StatusUnauthorized, "missing bearer token")
			return
		}
		claims, err := s.auth.Parse(token[len(prefix):])
		if err != nil {
			writeErr(w, http.StatusUnauthorized, "invalid token")
			return
		}
		next(w, r.WithContext(withClaims(r.Context(), claims)))
	}
}

func withClaims(ctx context.Context, c *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func claimsFrom(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(claimsKey).(*auth.Claims)
	return c
}