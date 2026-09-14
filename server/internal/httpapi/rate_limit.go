package httpapi

import (
	"sync"
	"time"
)

// rateLimiter — простой скользящий лимит: не более burst событий в минуту на пользователя.
type rateLimiter struct {
	mu      sync.Mutex
	last    map[int64]time.Time
	window  time.Duration
	burst   int
	allowed map[int64]int
}

func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		window:  time.Minute,
		burst:   60,
		last:    make(map[int64]time.Time),
		allowed: make(map[int64]int),
	}
}

// Allow решает, можно ли пользователю выполнить действие сейчас.
func (r *rateLimiter) Allow(userID int64) bool {
	r.mu.Lock()
	defer r.mu.Unlock()
	now := time.Now()
	if prev, ok := r.last[userID]; ok && now.Sub(prev) < r.window {
		if r.allowed[userID] >= r.burst {
			return false
		}
		r.allowed[userID]++
		return true
	}
	// новое окно
	r.last[userID] = now
	r.allowed[userID] = 1
	return true
}