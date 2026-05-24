//go:build !e2etest

package main

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"jamsesh/internal/portal/router"
)

// TestProductionBuild_HasNoTestEndpoint is the CI-level guardrail against
// build-tag drift. It runs only on production builds (no -tags e2etest)
// and asserts:
//
//  1. The production newTestClockProvider() returns a provider whose
//     mountTestEndpointsHook() is nil.
//  2. A router built with router.Deps{MountTest: nil} does not register
//     POST /test/clock-advance and returns 404.
//
// If a future contributor accidentally compiles the e2etest test endpoint
// into production builds, this test fires.
func TestProductionBuild_HasNoTestEndpoint(t *testing.T) {
	provider := newTestClockProvider()
	if provider.mountTestEndpointsHook() != nil {
		t.Fatalf("mountTestEndpointsHook() returned non-nil in production build")
	}
	if provider.magicLinkClock() != nil {
		t.Fatalf("magicLinkClock() returned non-nil in production build")
	}

	handler := router.New(router.Deps{
		Mounts: router.Mounts{
			Test: provider.mountTestEndpointsHook(),
		},
	})

	req := httptest.NewRequest(http.MethodPost, "/test/clock-advance",
		strings.NewReader(`{"advance_seconds": 60}`))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	handler.ServeHTTP(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Fatalf("POST /test/clock-advance status = %d, want 404 (production build must not expose the route)", rec.Code)
	}
}
