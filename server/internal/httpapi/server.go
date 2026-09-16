package httpapi

import (
	"context"
	"encoding/json"
	"net/http"

	"tgcloud/server/internal/auth"
	"tgcloud/server/internal/store"
	"tgcloud/server/internal/tgbot"
)

// Server — корневая структура HTTP-API. Хранит все зависимости: репозитории,
// менеджер авторизации, TG-бот, rate-limiter, метрики и путь к файлам загрузок.
type Server struct {
	repos         *store.Repos
	auth          *auth.Manager
	feedChannelID int64
	bot           *tgbot.Client
	rl            *rateLimiter
	metrics       *metrics
	uploadDir     string
	httpAddr      string
}

// New — конструктор Server. Инициализирует rate-limiter и метрики.
// feedChannelID — TG-чат основной ленты; bot — клиент Telegram Bot API;
// uploadDir — директория для загрузок; httpAddr — адрес для ListenAndServe.
func New(repos *store.Repos, am *auth.Manager, feedChannelID int64, bot *tgbot.Client, uploadDir, httpAddr string) *Server {
	return &Server{
		repos:         repos,
		auth:          am,
		feedChannelID: feedChannelID,
		bot:           bot,
		rl:            newRateLimiter(),
		metrics:       newMetrics(),
		uploadDir:     uploadDir,
		httpAddr:      httpAddr,
	}
}

// Routes — собирает и возвращает http.Handler со всеми маршрутами API.
// Публичные эндпоинты (/health, /metrics, /v1/auth/telegram) доступны без токена.
// Остальные обёрнуты в authMW; админ-эндпоинты дополнительно обёрнуты в adminMW.
// Гонко-чувствительные эндпоинты (like, comment, report) обёрнуты в rlMW.
// Внешний слой — requestLog (логирование + метрики) и corsMW (CORS-заголовки).
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
	mux.HandleFunc("GET /v1/shops/search", s.authMW(s.handleShopsSearch))
	mux.HandleFunc("GET /v1/shops/me", s.authMW(s.handleShopsMine))
	mux.HandleFunc("GET /v1/catalog", s.authMW(s.handleCatalog))
	mux.HandleFunc("GET /v1/shop/{id}", s.authMW(s.handleShopGet))
	mux.HandleFunc("PUT /v1/shop/{id}", s.authMW(s.handleShopUpdate))
	mux.HandleFunc("DELETE /v1/shop/{id}", s.authMW(s.handleShopDelete))
	mux.HandleFunc("POST /v1/shops/{id}/subscribe", s.authMW(s.handleShopSubscribe))
	mux.HandleFunc("DELETE /v1/shops/{id}/subscribe", s.authMW(s.handleShopUnsubscribe))
	mux.HandleFunc("GET /v1/me/subscriptions", s.authMW(s.handleMySubscriptions))

	mux.HandleFunc("POST /v1/product", s.authMW(s.handleProductCreate))
	mux.HandleFunc("GET /v1/product/{id}", s.authMW(s.handleProductGet))
	mux.HandleFunc("PUT /v1/product/{id}", s.authMW(s.handleProductUpdate))
	mux.HandleFunc("DELETE /v1/product/{id}", s.authMW(s.handleProductDelete))
	mux.HandleFunc("POST /v1/product/{id}/view", s.authMW(s.handleProductView))
	mux.HandleFunc("POST /v1/product/{id}/review", s.authMW(s.rlMW(s.handleReviewAdd)))
	mux.HandleFunc("GET /v1/product/{id}/reviews", s.authMW(s.handleReviewsByProduct))

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
	mux.HandleFunc("GET /v1/admin/comments", s.authMW(s.adminMW(s.handleAdminCommentsList)))
	mux.HandleFunc("POST /v1/admin/users/{id}/ban", s.authMW(s.adminMW(s.handleAdminUserBan)))
	mux.HandleFunc("POST /v1/admin/users/{id}/unban", s.authMW(s.adminMW(s.handleAdminUserUnban)))
	mux.HandleFunc("POST /v1/admin/shops/{id}/suspend", s.authMW(s.adminMW(s.handleAdminShopSuspend)))
	mux.HandleFunc("GET /v1/admin/filter-words", s.authMW(s.adminMW(s.handleFilterWordsList)))
	mux.HandleFunc("POST /v1/admin/filter-word", s.authMW(s.adminMW(s.handleFilterWordAdd)))
	mux.HandleFunc("DELETE /v1/admin/filter-word/{word}", s.authMW(s.adminMW(s.handleFilterWordRemove)))

	mux.HandleFunc("POST /v1/upload", s.authMW(s.handleUpload))
	if s.uploadDir != "" {
		mux.Handle("/uploads/", http.StripPrefix("/uploads/", http.FileServer(http.Dir(s.uploadDir))))
	}

	return s.requestLog(corsMW(mux))
}

// --- helpers ---

// writeJSON — сериализует v в JSON и записывает в ответ с указанным HTTP-кодом.
func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	_ = json.NewEncoder(w).Encode(v)
}

// writeErr — вспомогательный: формирует JSON {"error": msg} с указанным кодом.
func writeErr(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}

// ctxKey — тип-ключ для хранения claims в context.Context (избегаем коллизий ключей).
type ctxKey int

// claimsKey — константный ключ для извлечения auth.Claims из контекста запроса.
const claimsKey ctxKey = 0

// authMW — middleware авторизации. Извлекает Bearer-токен из заголовка Authorization,
// парсит его через auth.Manager, проверяет бан пользователя, кладёт claims в контекст.
// При ошибке возвращает 401 Unauthorized или 403 Forbidden (ban).
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
		// Проверяем, не забанен ли пользователь после выдачи токена.
		if u, err := s.repos.Users.Get(r.Context(), claims.UserID); err == nil && u.Banned {
			writeErr(w, http.StatusForbidden, "banned")
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

// withClaims — кладёт auth.Claims в контекст запроса для последующих middleware/хендлеров.
func withClaims(ctx context.Context, c *auth.Claims) context.Context {
	return context.WithValue(ctx, claimsKey, c)
}

// claimsFrom — извлекает auth.Claims из контекста; возвращает nil если не найдены.
func claimsFrom(ctx context.Context) *auth.Claims {
	c, _ := ctx.Value(claimsKey).(*auth.Claims)
	return c
}
