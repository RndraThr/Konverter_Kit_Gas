package distribution

import (
	"testing"
	"time"
)

func TestEffectiveCompletionTimeKeepsOfflineDayWithinLimits(t *testing.T) {
	now := time.Date(2026, 10, 15, 2, 0, 0, 0, time.UTC)
	yesterday := now.Add(-20 * time.Hour)
	future := now.Add(time.Hour)
	tooOld := now.Add(-8 * 24 * time.Hour)
	for _, tc := range []struct {
		name    string
		claimed *time.Time
		want    time.Time
	}{
		{"none uses server time", nil, now},
		{"offline completion keeps its own time", &yesterday, yesterday},
		{"phone clock ahead falls back to server time", &future, now},
		{"older than a week falls back to server time", &tooOld, now},
	} {
		if got := effectiveCompletionTime(tc.claimed, now); !got.Equal(tc.want) {
			t.Fatalf("%s: got %s want %s", tc.name, got, tc.want)
		}
	}
}
