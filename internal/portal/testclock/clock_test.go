//go:build e2etest

package testclock

import (
	"sync"
	"testing"
	"time"
)

func TestAdvanceableClock_StartsAtZeroOffset(t *testing.T) {
	c := New()
	if got := c.Offset(); got != 0 {
		t.Fatalf("Offset() = %v, want 0", got)
	}
}

func TestAdvanceableClock_AdvancePositive(t *testing.T) {
	c := New()
	got := c.Advance(60 * time.Second)
	if got != 60*time.Second {
		t.Fatalf("Advance(60s) returned %v, want 60s", got)
	}
	if c.Offset() != 60*time.Second {
		t.Fatalf("Offset() after Advance(60s) = %v, want 60s", c.Offset())
	}
}

func TestAdvanceableClock_AdvanceCumulative(t *testing.T) {
	c := New()
	_ = c.Advance(30 * time.Second)
	got := c.Advance(45 * time.Second)
	if got != 75*time.Second {
		t.Fatalf("cumulative Advance = %v, want 75s", got)
	}
}

func TestAdvanceableClock_AdvanceZeroIsNoOp(t *testing.T) {
	c := New()
	_ = c.Advance(10 * time.Second)
	got := c.Advance(0)
	if got != 10*time.Second {
		t.Fatalf("Advance(0) = %v, want 10s (unchanged)", got)
	}
}

func TestAdvanceableClock_AdvanceNegativeIsRejected(t *testing.T) {
	c := New()
	_ = c.Advance(30 * time.Second)
	got := c.Advance(-10 * time.Second)
	if got != 30*time.Second {
		t.Fatalf("Advance(-10s) = %v, want 30s (unchanged)", got)
	}
	if c.Offset() != 30*time.Second {
		t.Fatalf("Offset() after negative Advance = %v, want 30s", c.Offset())
	}
}

func TestAdvanceableClock_NowReflectsOffset(t *testing.T) {
	c := New()
	before := time.Now().UTC()
	_ = c.Advance(time.Hour)
	got := c.Now()
	// Conservative bound: got should be at least 1h ahead of `before`.
	if got.Sub(before) < time.Hour {
		t.Fatalf("Now() = %v, expected at least %v ahead of %v", got, time.Hour, before)
	}
}

func TestAdvanceableClock_NowIsUTC(t *testing.T) {
	c := New()
	got := c.Now()
	if got.Location() != time.UTC {
		t.Fatalf("Now() location = %v, want UTC", got.Location())
	}
}

func TestAdvanceableClock_ConcurrentAdvanceAndNow(t *testing.T) {
	c := New()
	const goroutines = 16
	const iters = 100
	var wg sync.WaitGroup
	wg.Add(goroutines * 2)
	for i := 0; i < goroutines; i++ {
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				c.Advance(time.Millisecond)
			}
		}()
		go func() {
			defer wg.Done()
			for j := 0; j < iters; j++ {
				_ = c.Now()
			}
		}()
	}
	wg.Wait()
	want := time.Duration(goroutines*iters) * time.Millisecond
	if got := c.Offset(); got != want {
		t.Fatalf("Offset after concurrent advances = %v, want %v", got, want)
	}
}
