package objectstore

import "time"

// Clock is an injectable time source. Mirrors the shape used across the
// portal (events.Clock, auth.Clock, tokens.Clock) so *testclock.AdvanceableClock
// satisfies this interface without import coupling. Per-package types let a
// single advance propagate everywhere via structural typing.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }
