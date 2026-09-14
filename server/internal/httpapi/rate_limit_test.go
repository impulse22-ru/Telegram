package httpapi

import (
	"testing"
	"time"
)

func TestRateLimiterBurst(t *testing.T) {
	rl := newRateLimiter()
	rl.window = time.Minute
	for i := 0; i < rl.burst; i++ {
		if !rl.Allow(7) {
			t.Fatalf("request %d should be allowed", i)
		}
	}
	if rl.Allow(7) {
		t.Fatal("request beyond burst should be denied")
	}
}

func TestRateLimiterIndependentUsers(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < rl.burst; i++ {
		rl.Allow(1)
	}
	if rl.Allow(1) {
		t.Fatal("user 1 exhausted")
	}
	if !rl.Allow(2) {
		t.Fatal("other user should be allowed")
	}
}

func TestRateLimiterWindowReset(t *testing.T) {
	rl := newRateLimiter()
	for i := 0; i < rl.burst; i++ {
		rl.Allow(1)
	}
	if rl.Allow(1) {
		t.Fatal("should be denied")
	}
	rl.mu.Lock()
	rl.last[1] = time.Now().Add(-2 * rl.window)
	rl.mu.Unlock()
	if !rl.Allow(1) {
		t.Fatal("new window should allow")
	}
}