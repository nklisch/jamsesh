package playground

import "time"

// Clock is an injectable time source. Mirrors sessions.Clock and auth.Clock so
// a single *testclock.AdvanceableClock satisfies all of them. Per-package types
// avoid cross-package import coupling — structural typing carries the
// "advance once, move everywhere" property.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// RealClock returns the production wall-clock implementation. Production
// callers (cmd/portal/main.go) use this; tests inject a fake clock.
func RealClock() Clock { return realClock{} }
