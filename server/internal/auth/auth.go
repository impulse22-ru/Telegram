package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

// ErrInvalid — ошибка, возвращаемая при невалидном или просроченном JWT-токене.
// Используется как sentinel error: callers проверяют через errors.Is(err, ErrInvalid).
var ErrInvalid = errors.New("invalid token")

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
// Инкапсулирует HMAC-секрет (HS256) и время жизни токена.
// Используется для:
//   - Выдачи токенов при входе через Telegram (Issue).
//   - Проверки токенов в middleware авторизации (Parse).
type Manager struct {
	secret []byte        // HMAC-ключ для подписи HS256
	expiry time.Duration // время жизни access-токена (например, 15 минут)
}

// NewManager — конструктор менеджера JWT.
//
// Параметры:
//   - secret: строковый HMAC-ключ (рекомендуется >= 32 байт, хранится в env JWT_SECRET).
//   - expiry: время жизни токена (например, 15*time.Minute).
//
// Возвращает готовый к использованию Manager.
func NewManager(secret string, expiry time.Duration) *Manager {
	return &Manager{secret: []byte(secret), expiry: expiry}
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
