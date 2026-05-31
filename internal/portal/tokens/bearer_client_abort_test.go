package tokens_test

// bearer_client_abort_test.go tests the client-abort (request context
// cancelled) classification in BearerMiddleware.
//
// Design (mirrors auth_client_abort_test.go in githttp):
//   - Request context cancelled before svc.Validate returns → 499 (no 5xx, no ERROR).
//   - Real dep/store error with a live request context → 503 unchanged.

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// TestBearerMiddleware_CancelledContext_Returns499 verifies that when
// svc.Validate is called on a request whose context has already been cancelled,
// BearerMiddleware returns 499 (client closed request) rather than 503
// (dep.db_unavailable). The discrimination must be on r.Context().Err() != nil:
// a genuine store error on a live ctx must still be 503.
func TestBearerMiddleware_CancelledContext_Returns499(t *testing.T) {
	// Arrange: a Validate stub that returns a generic transient-looking error.
	// On a cancelled context the middleware must detect the cancellation and
	// return 499 without logging ERROR.
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			// Simulate what a real DB call returns when the ctx is cancelled.
			return nil, context.Canceled
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer some-token")

	// Pre-cancel the request context so r.Context().Err() != nil when the
	// default branch runs.
	cancelCtx, cancel := context.WithCancel(context.Background())
	cancel()
	r = r.WithContext(cancelCtx)

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Fatal("next handler should not have been called")
	}
	// Must NOT be 5xx — a client abort is not a server fault.
	if w.Code >= 500 {
		t.Errorf("want non-5xx (client abort = 499), got %d", w.Code)
	}
	if w.Code != 499 {
		t.Errorf("want 499 (client closed request), got %d", w.Code)
	}
}

// TestBearerMiddleware_LiveCtx_StoreError_Still503 verifies that a genuine
// dep/store error (not a context cancellation) on a live request context still
// returns 503 dep.db_unavailable. The client-abort gate must not suppress
// real dependency failures.
func TestBearerMiddleware_LiveCtx_StoreError_Still503(t *testing.T) {
	transient := errors.New("unexpected EOF from postgres")
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			// Live ctx, genuine store error — must stay 503.
			return nil, transient
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer some-token")
	// Use a live (not cancelled) context — just like the existing
	// TestBearerMiddleware_TransientDBError_TypedEnvelope test.

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Fatal("next handler should not have been called")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503 (dep.db_unavailable) for live-ctx store error, got %d", w.Code)
	}
}
