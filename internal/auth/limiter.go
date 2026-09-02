package auth

import (
	"sync"
	"time"
)

type loginAttempts struct {
	count   int
	resetAt time.Time
}

type LoginLimiter struct {
	mu       sync.Mutex
	max      int
	window   time.Duration
	attempts map[string]loginAttempts
}

func NewLoginLimiter(max int, window time.Duration) *LoginLimiter {
	return &LoginLimiter{
		max:      max,
		window:   window,
		attempts: make(map[string]loginAttempts),
	}
}

func (l *LoginLimiter) Allow(key string, now time.Time) bool {
	l.mu.Lock()
	defer l.mu.Unlock()

	for existingKey, attempt := range l.attempts {
		if !now.Before(attempt.resetAt) {
			delete(l.attempts, existingKey)
		}
	}

	attempt := l.attempts[key]
	if attempt.resetAt.IsZero() {
		attempt.resetAt = now.Add(l.window)
	}
	if attempt.count >= l.max {
		return false
	}
	attempt.count++
	l.attempts[key] = attempt
	return true
}

func (l *LoginLimiter) Reset(key string) {
	l.mu.Lock()
	defer l.mu.Unlock()
	delete(l.attempts, key)
}
