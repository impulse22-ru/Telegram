package auth

import (
	"errors"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

var ErrInvalid = errors.New("invalid token")

type Claims struct {
	UserID  int64  `json:"uid"`
	Role    string `json:"role"`
	TgUserID int64 `json:"tgid"`
	jwt.RegisteredClaims
}

type Manager struct {
	secret []byte
	expiry time.Duration
}

func NewManager(secret string, expiry time.Duration) *Manager {
	return &Manager{secret: []byte(secret), expiry: expiry}
}

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
	return jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.secret)
}

func (m *Manager) Parse(token string) (*Claims, error) {
	parsed, err := jwt.ParseWithClaims(token, &Claims{}, func(t *jwt.Token) (any, error) {
		if _, ok := t.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, ErrInvalid
		}
		return m.secret, nil
	})
	if err != nil {
		return nil, err
	}
	claims, ok := parsed.Claims.(*Claims)
	if !ok || !parsed.Valid {
		return nil, ErrInvalid
	}
	return claims, nil
}