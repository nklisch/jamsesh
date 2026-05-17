//go:build !e2etest

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
	"jamsesh/internal/portal/tokens"
)

// testClockProvider is a no-op stub in production builds. The presence
// of two files with mutually-exclusive build tags ensures exactly one
// definition compiles. See test_clock_advance.go for the e2etest variant.
type testClockProvider struct{}

func newTestClockProvider() *testClockProvider { return &testClockProvider{} }

// magicLinkClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to auth.NewMagicLinkHandler. The return type
// is the concrete auth.Clock interface so the comparison against nil
// in main.go is well-defined (no typed-nil trap).
func (p *testClockProvider) magicLinkClock() auth.Clock { return nil }

// tokensClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to tokens.New. The return type is the concrete
// tokens.Clock interface so the comparison against nil in main.go is
// well-defined (no typed-nil trap).
func (p *testClockProvider) tokensClock() tokens.Clock { return nil }

// accountsClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to accounts.New. The return type is the concrete
// accounts.Clock interface so the comparison against nil in main.go is
// well-defined (no typed-nil trap).
func (p *testClockProvider) accountsClock() accounts.Clock { return nil }

// commentsClock returns nil. main.go assigns the nil into the Clock
// field on the comments.Service struct literal; the service's now()
// helper falls back to the real wall clock when Clock is nil. The
// return type is the concrete comments.Clock interface so the field
// assignment is well-typed.
func (p *testClockProvider) commentsClock() comments.Clock { return nil }

// finalizeClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to finalize.New. The return type is the
// concrete finalize.Clock interface so the comparison against nil in
// main.go is well-defined (no typed-nil trap).
func (p *testClockProvider) finalizeClock() finalize.Clock { return nil }

// storageClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to storage.New. The return type is the concrete
// storage.Clock interface so the comparison against nil in main.go is
// well-defined (no typed-nil trap).
func (p *testClockProvider) storageClock() storage.Clock { return nil }

// eventsClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to events.New. The return type is the concrete
// events.Clock interface so the comparison against nil in main.go is
// well-defined (no typed-nil trap).
func (p *testClockProvider) eventsClock() events.Clock { return nil }

// automergerClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to automerger.NewApplier. The return type is
// the concrete automerger.Clock interface so the comparison against
// nil in main.go is well-defined (no typed-nil trap).
func (p *testClockProvider) automergerClock() automerger.Clock { return nil }

// mcpClock returns nil. main.go assigns the nil into the Clock field
// on the mcpendpoint.Endpoint struct literal; the endpoint's now()
// helper falls back to the real wall clock when Clock is nil. The
// return type is the concrete mcpendpoint.Clock interface so the
// field assignment is well-typed.
func (p *testClockProvider) mcpClock() mcpendpoint.Clock { return nil }

// sessionsClock returns nil. main.go interprets nil as "use the real
// clock" and falls back to sessions.New. The return type is the
// concrete sessions.Clock interface so the comparison against nil in
// main.go is well-defined (no typed-nil trap).
func (p *testClockProvider) sessionsClock() sessions.Clock { return nil }

// mountTestEndpointsHook returns nil in production builds. router.New
// skips the /test r.Route call when MountTest is nil, so the /test
// subtree is never registered in production binaries.
func (p *testClockProvider) mountTestEndpointsHook() func(chi.Router) {
	return nil
}
