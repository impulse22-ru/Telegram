package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"tgcloud/server/internal/auth"
	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

type Server struct {
	repos         *store.Repos
	auth          *auth.Manager
	feedChannelID int64
	bot           *tgbot.Client
	rl            *rateLimiter
	metrics       *metrics
}

func New(repos *store.Repos, am *auth.Manager, feedChannelID int64, bot *tgbot.Client) *Server {
	return &Server{
		repos:         repos,
		auth:          am,
		feedChannelID: feedChannelID,
		bot:           bot,
		rl:            newRateLimiter(),
		metrics:       newMetrics(),
	}
}

func (s *Server) Routes() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /health", s.handleHealth)
	mux.HandleFunc("GET /metrics", s.handleMetrics)
	mux.HandleFunc("POST /v1/auth/telegram", s.handleAuthTelegram)

	mux.HandleFunc("GET /v1/feed", s.authMW(s.handleFeed))
	mux.HandleFunc("GET /v1/search", s.authMW(s.handleSearch))
	mux.HandleFunc("GET /v1/videos/{id}", s.authMW(s.handleVideoGet))
	mux.HandleFunc("GET /v1/videos/{id}/stats", s.authMW(s.handleVideoStats))
	mux.HandleFunc("POST /v1/videos/{id}/like", s.authMW(s.rlMW(s.handleLike)))
	mux.HandleFunc("POST /v1/videos/{id}/unlike", s.authMW(s.rlMW(s.handleUnlike)))
	mux.HandleFunc("GET /v1/videos/{id}/comments", s.authMW(s.handleCommentsList))
	mux.HandleFunc("POST /v1/videos/{id}/comment", s.authMW(s.rlMW(s.handleComment)))
	mux.HandleFunc("POST /v1/videos/{id}/report", s.authMW(s.rlMW(s.handleReport)))
	mux.HandleFunc("POST /v1/videos/{id}/view", s.authMW(s.handleView))
	mux.HandleFunc("POST /v1/join", s.authMW(s.handleJoin))

	mux.HandleFunc("POST /v1/shop", s.authMW(s.handleShopCreate))
	mux.HandleFunc("GET /v1/shops", s.authMW(s.handleShopsList))
	mux.HandleFunc("GET /v1/shops/me", s.authMW(s.handleShopsMine))
	mux.HandleFunc("GET /v1/catalog", s.authMW(s.handleCatalog))
	mux.HandleFunc("GET /v1/shop/{id}", s.authMW(s.handleShopGet))
	mux.HandleFunc("POST /v1/shops/{id}/subscribe", s.authMW(s.handleShopSubscribe))
	mux.HandleFunc("DELETE /v1/shops/{id}/subscribe", s.authMW(s.handleShopUnsubscribe))
	mux.HandleFunc("GET /v1/me/subscriptions", s.authMW(s.handleMySubscriptions))

	mux.HandleFunc("POST /v1/product", s.authMW(s.handleProductCreate))
	mux.HandleFunc("GET /v1/product/{id}", s.authMW(s.handleProductGet))
	mux.HandleFunc("POST /v1/product/{id}/view", s.authMW(s.handleProductView))

	mux.HandleFunc("POST /v1/order", s.authMW(s.handleOrderCreate))
	mux.HandleFunc("GET /v1/order/{id}", s.authMW(s.handleOrderGet))
	mux.HandleFunc("POST /v1/order/{id}/pay", s.authMW(s.handleOrderPay))
	mux.HandleFunc("POST /v1/order/{id}/cancel", s.authMW(s.handleOrderCancel))
	mux.HandleFunc("POST /v1/order/{id}/confirm", s.authMW(s.handleOrderConfirm))
	mux.HandleFunc("GET /v1/order/{id}/chat", s.authMW(s.handleOrderChat))
	mux.HandleFunc("GET /v1/orders/seller", s.authMW(s.handleOrdersSeller))
	mux.HandleFunc("GET /v1/orders/me", s.authMW(s.handleOrdersMine))

	mux.HandleFunc("GET /v1/stats/me", s.authMW(s.handleMyStats))
	mux.HandleFunc("GET /v1/stats/sales", s.authMW(s.handleSalesStats))

	mux.HandleFunc("GET /v1/admin/stats", s.authMW(s.adminMW(s.handleAdminStats)))
	mux.HandleFunc("GET /v1/admin/top", s.authMW(s.adminMW(s.handleAdminTop)))
	mux.HandleFunc("GET /v1/admin/reports", s.authMW(s.adminMW(s.handleAdminReports)))
	mux.HandleFunc("POST /v1/admin/reports/{id}/status", s.authMW(s.adminMW(s.handleAdminReportStatus)))
	mux.HandleFunc("POST /v1/admin/videos/{id}/ban", s.authMW(s.adminMW(s.handleAdminVideoBan)))
	mux.HandleFunc("POST /v1/admin/videos/{id}/unban", s.authMW(s.adminMW(s.handleAdminVideoUnban)))
	mux.HandleFunc("DELETE /v1/admin/videos/{id}", s.authMW(s.adminMW(s.handleAdminVideoDelete)))
	mux.HandleFunc("DELETE /v1/admin/comments/{id}", s.authMW(s.adminMW(s.handleAdminCommentDelete)))
	mux.HandleFunc("POST /v1/admin/shops/{id}/suspend", s.authMW(s.adminMW(s.handleAdminShopSuspend)))
	mux.HandleFunc("GET /v1/admin/filter-words", s.authMW(s.adminMW(s.handleFilterWordsList)))
	mux.HandleFunc("POST /v1/admin/filter-word", s.authMW(s.adminMW(s.handleFilterWordAdd)))
	mux.HandleFunc("DELETE /v1/admin/filter-word/{word}", s.authMW(s.adminMW(s.handleFilterWordRemove)))

	return s.requestLog(mux)
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

// rlMW — простейший неаккуратный rate-limit по пользователю (per-user leaky bucket).
func (s *Server) rlMW(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFrom(r.Context())
		if claims != nil && !s.rl.Allow(claims.UserID) {
			writeErr(w, http.StatusTooManyRequests, "rate limited")
			return
		}
		next(w, r)
	}
}

// adminMW — проверки роли admin.
func (s *Server) adminMW(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		claims := claimsFrom(r.Context())
		if claims == nil || claims.Role != "admin" {
			writeErr(w, http.StatusForbidden, "admin only")
			return
		}
		next(w, r)
	}
}

func withClaims(ctx context.Context, c *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

func claimsFrom(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(claimsKey).(*auth.Claims)
	return c
}