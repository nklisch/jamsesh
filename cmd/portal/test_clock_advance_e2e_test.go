//go:build e2etest

package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/portal/router"
)

// TestE2EBuild_TestEndpointMounted boots the same router production
// uses, but with the e2etest-tagged testClockProvider supplying a
// non-nil MountTest hook. Asserts the happy path and the 400 paths.
func TestE2EBuild_TestEndpointMounted(t *testing.T) {
	provider := newTestClockProvider()
	if hook := provider.mountTestEndpointsHook(); hook == nil {
		t.Fatalf("mountTestEndpointsHook() returned nil in e2etest build")
	}
	if clock := provider.magicLinkClock(); clock == nil {
		t.Fatalf("magicLinkClock() returned nil in e2etest build")
	}

	handler := router.New(router.Deps{
		Mounts: router.Mounts{
			Test: provider.mountTestEndpointsHook(),
		},
	})

	t.Run("happy_path", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test/clock-advance",
			strings.NewReader(`{"advance_seconds": 900}`))
		req.Header.Set("Content-Type", "application/json")
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)

		if rec.Code != http.StatusOK {
			t.Fatalf("status = %d, want 200, body = %q", rec.Code, rec.Body.String())
		}
		var body struct {
			Now           string `json:"now"`
			OffsetSeconds int64  `json:"offset_seconds"`
		}
		if err := json.NewDecoder(rec.Body).Decode(&body); err != nil {
			t.Fatalf("decode response: %v", err)
		}
		if body.OffsetSeconds != 900 {
			t.Fatalf("offset_seconds = %d, want 900", body.OffsetSeconds)
		}
		if _, err := time.Parse(time.RFC3339Nano, body.Now); err != nil {
			t.Fatalf("now = %q is not RFC3339Nano: %v", body.Now, err)
		}
	})

	t.Run("negative_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test/clock-advance",
			strings.NewReader(`{"advance_seconds": -5}`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("negative status = %d, want 400", rec.Code)
		}
	})

	t.Run("invalid_json_400", func(t *testing.T) {
		req := httptest.NewRequest(http.MethodPost, "/test/clock-advance",
			strings.NewReader(`not json`))
		rec := httptest.NewRecorder()
		handler.ServeHTTP(rec, req)
		if rec.Code != http.StatusBadRequest {
			t.Fatalf("invalid-json status = %d, want 400", rec.Code)
		}
	})
}
