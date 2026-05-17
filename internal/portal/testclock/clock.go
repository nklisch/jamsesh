//go:build e2etest

// Package testclock provides an advanceable clock for e2e tests.
//
// Every file in this package carries the //go:build e2etest tag, so
// production builds (go build with no tags) cannot import it — doing
// so is a compile error, which is the desired structural guard against
// shipping a clock-manipulation surface in production binaries.
//
// The clock is process-global by design: once advanced, the offset
// cannot be rewound. Tests that need a known offset must reason about
// deltas against the response's offset_seconds field, or boot a fresh
// portal container.
//
// See feature portal-test-clock-advance-endpoint for context.
package testclock

import (
	"sync"
	"time"
)

// AdvanceableClock is a process-global clock that returns
// time.Now().UTC() plus a cumulative forward-only offset. Safe for
// concurrent use.
//
// The shape (Now() time.Time) intentionally matches both
// internal/portal/auth.Clock and internal/portal/tokens.Clock so a
// single AdvanceableClock instance can satisfy both interfaces.
type AdvanceableClock struct {
	mu     sync.Mutex
	offset time.Duration
}

// New returns a fresh AdvanceableClock with zero offset.
func New() *AdvanceableClock { return &AdvanceableClock{} }

// Now returns the current wall time plus the accumulated offset, in UTC.
func (c *AdvanceableClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return time.Now().UTC().Add(c.offset)
}

// Advance adds d to the cumulative offset and returns the new
// cumulative offset. If d <= 0 the offset is unchanged and the current
// offset is returned (callers that pass zero get a no-op read).
//
// Rewind is intentionally not supported: the clock is process-global,
// and rewinding would create cross-test contamination.
func (c *AdvanceableClock) Advance(d time.Duration) time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	if d > 0 {
		c.offset += d
	}
	return c.offset
}

// Offset returns the current cumulative offset.
func (c *AdvanceableClock) Offset() time.Duration {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.offset
}
