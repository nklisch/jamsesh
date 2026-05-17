//go:build !e2etest

package main

import (
	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/auth"
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

// mountTestEndpointsHook returns nil in production builds. router.New
// skips the /test r.Route call when MountTest is nil, so the /test
// subtree is never registered in production binaries.
func (p *testClockProvider) mountTestEndpointsHook() func(chi.Router) {
	return nil
}
