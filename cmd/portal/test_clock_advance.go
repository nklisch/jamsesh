//go:build e2etest

package main

import (
	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/accounts"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/mcpendpoint"
	"jamsesh/internal/portal/sessions"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/testclock"
	"jamsesh/internal/portal/tokens"
)

// testClockProvider holds the singleton AdvanceableClock used by
// e2etest-tagged builds. Constructed once at server start; injected
// into every handler that needs time.Now indirection (magic-link AND
// tokens — advancing once moves both forward).
//
// This file's mirror, test_clock_advance_prod.go, carries the
// //go:build !e2etest tag and provides a no-op stub. Exactly one of
// the two compiles per build, by mutual exclusion of build tags.
type testClockProvider struct {
	clock *testclock.AdvanceableClock
}

func newTestClockProvider() *testClockProvider {
	return &testClockProvider{clock: testclock.New()}
}

// magicLinkClock returns the clock to inject into the magic-link
// handler. Implements auth.Clock.
func (p *testClockProvider) magicLinkClock() auth.Clock { return p.clock }

// tokensClock returns the clock to inject into the tokens.Service.
// Implements tokens.Clock. Same underlying AdvanceableClock as
// magicLinkClock — advancing once moves both forward.
func (p *testClockProvider) tokensClock() tokens.Clock { return p.clock }

// accountsClock returns the clock to inject into the accounts.Handler.
// Implements accounts.Clock. Same underlying AdvanceableClock as
// magicLinkClock / tokensClock — advancing once moves all forward.
func (p *testClockProvider) accountsClock() accounts.Clock { return p.clock }

// commentsClock returns the clock to inject into the comments.Service.
// Implements comments.Clock. Same shared AdvanceableClock as every other
// accessor.
func (p *testClockProvider) commentsClock() comments.Clock { return p.clock }

// finalizeClock returns the clock to inject into the finalize.Handler.
// Implements finalize.Clock. Same shared AdvanceableClock as every other
// accessor — so /test/clock-advance moves the 30-minute idle-lock window.
func (p *testClockProvider) finalizeClock() finalize.Clock { return p.clock }

// storageClock returns the clock to inject into the storage.Service.
// Implements storage.Clock. Same shared AdvanceableClock as every other
// accessor.
func (p *testClockProvider) storageClock() storage.Clock { return p.clock }

// eventsClock returns the clock to inject into the events.Log.
// Implements events.Clock. Same shared AdvanceableClock as every other
// accessor.
func (p *testClockProvider) eventsClock() events.Clock { return p.clock }

// automergerClock returns the clock to inject into the automerger.Applier.
// Implements automerger.Clock. Same shared AdvanceableClock — advancing
// affects the merger signature timestamp and conflict event/resolve
// timestamps for background-merge readers.
func (p *testClockProvider) automergerClock() automerger.Clock { return p.clock }

// mcpClock returns the clock to inject into the mcpendpoint.Endpoint.
// Implements mcpendpoint.Clock. Same shared AdvanceableClock — affects
// the sentinel TokenInfo.Expiration stamp and the fork tool's ForkedAt
// payload.
func (p *testClockProvider) mcpClock() mcpendpoint.Clock { return p.clock }

// sessionsClock returns the clock to inject into the sessions.Handler.
// Implements sessions.Clock. Same shared AdvanceableClock — affects
// session create/abandon stamps, invite create/expires/accept/join
// stamps, and the ListSessions cursor "before" window.
func (p *testClockProvider) sessionsClock() sessions.Clock { return p.clock }

// mountTestEndpoints registers POST /clock-advance on r. The portal
// router invokes this inside r.Route("/test", ...), so the public
// surface becomes POST /test/clock-advance. Non-POST methods on the
// path are rejected by chi's standard MethodNotAllowed handling.
func (p *testClockProvider) mountTestEndpoints(r chi.Router) {
	r.Method("POST", "/clock-advance", testclock.RouteMount(p.clock))
}

// mountTestEndpointsHook returns the chi.Router mount function for
// router.Deps.MountTest. The e2etest-tagged variant returns a real
// callable; the production stub returns nil so router.New skips the
// /test subtree entirely.
func (p *testClockProvider) mountTestEndpointsHook() func(chi.Router) {
	return p.mountTestEndpoints
}
