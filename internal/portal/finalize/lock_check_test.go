package finalize

import (
	"testing"
	"time"
)

func TestIsLockExpired(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)

	cases := []struct {
		name         string
		lastActivity time.Time
		want         bool
	}{
		{
			name:         "well-before TTL — not expired",
			lastActivity: now.Add(-1 * time.Minute),
			want:         false,
		},
		{
			name:         "exact TTL boundary — not expired (strict >)",
			lastActivity: now.Add(-FinalizeLockTTL),
			want:         false,
		},
		{
			name:         "one ns past TTL — expired",
			lastActivity: now.Add(-FinalizeLockTTL - time.Nanosecond),
			want:         true,
		},
		{
			name:         "well past TTL — expired",
			lastActivity: now.Add(-2 * time.Hour),
			want:         true,
		},
		{
			name:         "just-before-TTL (29:59) — not expired",
			lastActivity: now.Add(-29*time.Minute - 59*time.Second),
			want:         false,
		},
		{
			name:         "just-after-TTL (30:01) — expired",
			lastActivity: now.Add(-30*time.Minute - 1*time.Second),
			want:         true,
		},
		{
			name:         "clock-skewed-backwards (future last_activity) — not expired",
			lastActivity: now.Add(1 * time.Minute),
			want:         false,
		},
		{
			name:         "exact equality (last_activity == now) — not expired",
			lastActivity: now,
			want:         false,
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := IsLockExpired(tc.lastActivity, now)
			if got != tc.want {
				t.Errorf("IsLockExpired(lastActivity=%v, now=%v) = %v, want %v",
					tc.lastActivity, now, got, tc.want)
			}
		})
	}
}

func TestLockExpiresAt(t *testing.T) {
	now := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	got := LockExpiresAt(now)
	want := now.Add(30 * time.Minute)
	if !got.Equal(want) {
		t.Errorf("LockExpiresAt(%v) = %v, want %v", now, got, want)
	}
}

func TestFinalizeLockTTLValue(t *testing.T) {
	// Lock the constant value at the package level so a refactor that
	// silently changes the TTL (e.g. picking up a config knob by accident)
	// trips this test. The TTL is locked at epic-design.
	if FinalizeLockTTL != 30*time.Minute {
		t.Fatalf("FinalizeLockTTL = %v, want 30m", FinalizeLockTTL)
	}
}
