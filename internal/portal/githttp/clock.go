// Package githttp — clock injection.
//
// Per-package clock interface so a *testclock.AdvanceableClock satisfies both
// playground.Clock and githttp.Clock structurally without import coupling. See
// .claude/skills/patterns/per-package-clock-interface.md.
package githttp

import "time"

// Clock is an injectable time source. Mirrors playground.Clock so a single
// *testclock.AdvanceableClock satisfies both without import coupling.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// RealClock returns the production wall-clock implementation.
func RealClock() Clock { return realClock{} }
