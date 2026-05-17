package mcpendpoint_test

import (
	"testing"
	"time"

	"jamsesh/internal/portal/mcpendpoint"
)

// fakeClock is a controllable time source used to exercise the clock-
// injection path on the mcpendpoint.Endpoint. Mirrors the shape of
// fakeClock in internal/portal/auth/magic_link_test.go.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time { return f.t }

// TestEndpoint_NilClockFallsBackToReal asserts that an Endpoint
// constructed without a Clock field still constructs cleanly — the
// now() helper falls back to the real wall clock when Clock is nil.
// Smoke test that preserves the struct-literal contract used by
// cmd/portal/main.go.
func TestEndpoint_NilClockFallsBackToReal(t *testing.T) {
	env := newTestEnv(t)

	// Build a fresh endpoint with NO Clock field set.
	endpoint := &mcpendpoint.Endpoint{
		Store:    env.s,
		Tokens:   env.tokens,
		Storage:  env.storage,
		Log:      env.log,
		Comments: env.svc,
	}

	// Confirm construction with nil Clock doesn't panic — the deferred
	// clock read happens at request time, not construction time. The
	// SDK's TokenInfo.Expiration field is filled by verifyToken at
	// request time via e.now(); we just need the struct to build.
	if h := endpoint.Handler(); h == nil {
		t.Fatal("Handler() returned nil with nil Clock")
	}
}

// TestEndpoint_InjectedClock_StructFieldRoundtrip asserts that the
// Clock field set via struct-literal initialization is reachable. This
// is the same wiring main.go uses to thread the e2etest AdvanceableClock
// into the endpoint.
func TestEndpoint_InjectedClock_StructFieldRoundtrip(t *testing.T) {
	env := newTestEnv(t)

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: fixed}

	endpoint := &mcpendpoint.Endpoint{
		Store:    env.s,
		Tokens:   env.tokens,
		Storage:  env.storage,
		Log:      env.log,
		Comments: env.svc,
		Clock:    clk,
	}

	if got := endpoint.Clock.Now(); !got.Equal(fixed) {
		t.Errorf("Endpoint.Clock.Now(): want %v, got %v", fixed, got)
	}
}
