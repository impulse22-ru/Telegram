package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalid — ошибка, возвращаемая при невалидном или просроченном JWT-токене.
// Используется как sentinel error: callers проверяют через errors.Is(err, ErrInvalid).
var ErrInvalid = errors.New("invalid token")

// ErrNoRefresh — ошибка: refresh-токен не найден/недействителен (истёк, удалён).
var ErrNoRefresh = errors.New("refresh token not found")

// ErrNoSessionStore — ошибка: refresh-хранилище не привязано к менеджеру.
var ErrNoSessionStore = errors.New("session store not configured")

// SessionStore — хранилище refresh-сессий (opaque-токенов) для ротации и отзыва.
// Реализуется поверх Redis (см. store/sessions_repo.go): ключ — sha256
// от raw-токена, значение — id пользователя; TTL равен времени жизни refresh.
type SessionStore interface {
	// SaveRefresh — сохраняет активный refresh-токен (hash) для пользователя.
	SaveRefresh(ctx context.Context, refreshHash string, userID int64, ttl time.Duration) error
	// RefreshUserID — возвращает id пользователя по hash refresh-токена;
	// возвращает ErrNoRefresh если ключ отсутствует (невалидный/истёкший).
	RefreshUserID(ctx context.Context, refreshHash string) (int64, error)
	// DeleteRefresh — удаляет refresh-токен (отзыв/ротация/выход).
	DeleteRefresh(ctx context.Context, refreshHash string) error
}

// Claims — пользовательские поля JWT-токена (payload).
// Содержит идентификаторы пользователя в двух системах:
//   - UserID — внутренний ID в БД приложения (posts.users).
//   - TgUserID — Telegram user ID (уникален в Telegram, неизменен).
//   - Role — роль пользователя ("admin", "user" и т.д.).
//
// RegisteredClaims — стандартные поля JWT (exp, iat), встраиваемые через композицию.
type Claims struct {
	UserID   int64  `json:"uid"`
	Role     string `json:"role"`
	TgUserID int64  `json:"tgid"`
	jwt.RegisteredClaims
}

// Manager — менеджер JWT-токенов.
// Инкапсулирует HMAC-секрет (HS256), время жизни access-токена и
// интерфейс хранилища refresh-сессий (opaque-токены поверх Redis).
// Используется для:
//   - Выдачи токенов при входе через Telegram (Issue / IssueRefresh).
//   - Проверки токенов в middleware авторизации (Parse).
//   - Ротации/отзыва refresh-токенов (Rotate / Revoke).
type Manager struct {
	secret        []byte        // HMAC-ключ для подписи HS256
	expiry        time.Duration // время жизни access-токена (например, 15 минут)
	refreshExpiry time.Duration // время жизни refresh-токена (например, 30 дней)
	store         SessionStore  // Redis-хранилище refresh-сессий (nil = refresh выключен)
}

// NewManager — конструктор менеджера JWT.
//
// Параметры:
//   - secret: строковый HMAC-ключ (рекомендуется >= 32 байт, хранится в env JWT_SECRET).
//   - expiry: время жизни токена (например, 15*time.Minute).
//
// Refresh-функциональность подключается отдельно через WithRefresh.
// Возвращает готовый к использованию Manager.
func NewManager(secret string, expiry time.Duration) *Manager {
	return &Manager{secret: []byte(secret), expiry: expiry}
}

// WithRefresh — привязывает refresh-хранилище и время жизни refresh-токенов.
// Без вызова эндпоинты /v1/auth/refresh и /v1/auth/logout возвращают 503.
func (m *Manager) WithRefresh(refreshExpiry time.Duration, store SessionStore) *Manager {
	m.refreshExpiry = refreshExpiry
	m.store = store
	return m
}

// Issue — создание и подпись JWT-токена.
//
// Формирует payload с Claims (userID, tgUserID, role) и стандартными полями:
//   - exp (ExpiresAt) — текущее время + expiry.
//   - iat (IssuedAt) — текущее время.
//
// Подпись: HMAC-SHA256 (HS256) — симметричный алгоритм, секрет известен только серверу.
//
// Возвращает signed string (eyJhbGci...), который клиент передаёт в заголовке Authorization.
func (m *Manager) Issue(userID, tgUserID int64, role string) (string, error) {
	claims := Claims{
		UserID:   userID,
		Role:     role,
		TgUserID: tgUserID,
		RegisteredClaims: jwt.RegisteredClaims{
			ExpiresAt: jwt.NewNumericDate(time.Now().Add(m.expiry)),
			IssuedAt:  jwt.NewNumericDate(time.Now()),
		},
	}
	// jwt.NewWithClaims создаёт токен, SignedString подписывает его HMAC-ключом.
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

// Parse — парсинг и валидация JWT-токена.
//
// Алгоритм:
//  1. ParseWithClaims извлекает claims из токена.
//  2. Ключевая функция (keyFunc) проверяет, что алгоритм подписи — HMAC (защита от алгоритм-спуфинга: если
//     злоумышленник изменит header алгоритма на "none" или "RS256", проверка упадёт).
//  3. Если парсинг прошёл — извлекаем Claims и проверяем валидность (exp, iat).
//
// Возвращает *Claims (успешно) или ErrInvalid (невалидный/просроченный токен).
func (m *Manager) Parse(token string) (*Claims, error) {
	// keyFunc вызывается для каждого токена: возвращает ключ для проверки подписи.
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		// Защита от подмены алгоритма: принимаем только HMAC (HS256/HS384/HS512).
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err // включает.ValidationError (Expired, NotValidYet и т.д.)
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalid
	}
	return claims, nil
}

// IssueRefresh — выдаёт новый opaque refresh-токен и сохраняет его hash в хранилище.
// Raw-токен (32 случайных байта в hex) возвращается клиенту один раз;
// в Redis хранится только его sha256 — кража БД не раскрывает действующие сессии.
// Возвращает raw-токен или ErrNoSessionStore.
func (m *Manager) IssueRefresh(userID int64) (string, error) {
	if m.store == nil {
		return "", ErrNoSessionStore
	}
	raw := newRefreshToken()
	// Храним sha256(raw) — это и есть идентификатор сессии.
	if err := m.store.SaveRefresh(context.Background(), refreshHash(raw), userID, m.refreshExpiry); err != nil {
		return "", err
	}
	return raw, nil
}

// ValidateRefresh — проверяет refresh-токен по hash в хранилище.
// Возвращает id пользователя и nil при успехе, ErrNoRefresh если сессия
// отсутствует (истекла/отозвана) или ErrNoSessionStore если не настроена.
func (m *Manager) ValidateRefresh(raw string) (int64, error) {
	if m.store == nil {
		return 0, ErrNoSessionStore
	}
	return m.store.RefreshUserID(context.Background(), refreshHash(raw))
}

// Revoke — отзывает refresh-токен (удаляет hash из хранилища).
// Используется в /v1/auth/logout и перед ротацией.
func (m *Manager) Revoke(raw string) error {
	if m.store == nil {
		return ErrNoSessionStore
	}
	return m.store.DeleteRefresh(context.Background(), refreshHash(raw))
}

// Rotate — классическая ротация refresh-токена:
// старый отзывается, вместо него выдаётся новый (защита от повторного использования
// украденного токена). Возвращает новое значение refresh-токена.
func (m *Manager) Rotate(oldRaw string) (int64, string, error) {
	uid, err := m.ValidateRefresh(oldRaw)
	if err != nil {
		return 0, "", err
	}
	// Отзываем старую сессию, выдаём новую (одноразовость).
	if err := m.Revoke(oldRaw); err != nil {
		return 0, "", err
	}
	newRaw, err := m.IssueRefresh(uid)
	if err != nil {
		return 0, "", err
	}
	return uid, newRaw, nil
}

// newRefreshToken — 32 случайных байта в hex (256 бит энтропии).
func newRefreshToken() string {
	var b [32]byte
	if _, err := rand.Read(b[:]); err != nil {
		// Практически недостижимо; системный RNG не выходит из строя.
		return ""
	}
	return hex.EncodeToString(b[:])
}

// refreshHash — sha256 от raw-токена; используется как ключ сессии в Redis.
func refreshHash(raw string) string {
	h := sha256.Sum256([]byte(raw))
	return hex.EncodeToString(h[:])
}
