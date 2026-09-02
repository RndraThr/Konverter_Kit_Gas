package auth

import (
	"testing"
	"time"
)

func TestLoginLimiterRejectsSixthAttemptWithinWindow(t *testing.T) {
	limiter := NewLoginLimiter(5, 15*time.Minute)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	for attempt := 1; attempt <= 5; attempt++ {
		if !limiter.Allow("127.0.0.1|admin", now) {
			t.Fatalf("attempt %d should be allowed", attempt)
		}
	}
	if limiter.Allow("127.0.0.1|admin", now) {
		t.Fatal("sixth attempt should be rejected")
	}
}

func TestLoginLimiterResetsAfterWindowOrSuccessfulLogin(t *testing.T) {
	limiter := NewLoginLimiter(1, 15*time.Minute)
	now := time.Date(2026, 9, 2, 12, 0, 0, 0, time.UTC)
	if !limiter.Allow("key", now) || limiter.Allow("key", now) {
		t.Fatal("expected first attempt only")
	}
	if !limiter.Allow("key", now.Add(16*time.Minute)) {
		t.Fatal("expected attempt after the window")
	}
	limiter.Reset("key")
	if !limiter.Allow("key", now.Add(16*time.Minute)) {
		t.Fatal("expected attempt after explicit reset")
	}
}
