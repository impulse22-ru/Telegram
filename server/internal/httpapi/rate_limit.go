package httpapi

import (
	"sync"
	"time"
)

// rateLimiter — простой скользящий лимит: не более burst событий в минуту на пользователя.
// Хранит время начала текущего окна (last) и число разрешённых событий в нём (allowed)
// в двух параллельных map, защищённых одним mutex'ом.
type rateLimiter struct {
	mu      sync.Mutex
	last    map[int64]time.Time
	window  time.Duration
	burst   int
	allowed map[int64]int
}

// newRateLimiter — конструктор: окно 1 минута, лимит 60 событий на пользователя.
func newRateLimiter() *rateLimiter {
	return &rateLimiter{
		window:  time.Minute,
		burst:   60,
		last:    make(map[int64]time.Time),
		allowed: make(map[int64]int),
	}
}

// Allow решает, можно ли пользователю выполнить действие сейчас.
// Если окно (window) ещё не истекло — сравниваем число событий с burst;
// если истекло/окна нет — открываем новое окно и разрешаем первое событие.
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
