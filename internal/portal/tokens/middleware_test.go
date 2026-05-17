package tokens_test

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// mockService implements tokens.Service for middleware tests.
type mockService struct {
	validateFn func(ctx context.Context, raw string) (*store.Account, error)
}

func (m *mockService) Issue(_ context.Context, _ string) (tokens.Pair, error) {
	return tokens.Pair{}, errors.New("not implemented")
}

func (m *mockService) Validate(ctx context.Context, raw string) (*store.Account, error) {
	return m.validateFn(ctx, raw)
}

func (m *mockService) Refresh(_ context.Context, _ string) (tokens.Pair, error) {
	return tokens.Pair{}, errors.New("not implemented")
}

func (m *mockService) Revoke(_ context.Context, _ string, _ bool) error {
	return errors.New("not implemented")
}

func nextHandler(reached *bool) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*reached = true
		w.WriteHeader(http.StatusOK)
	})
}

func TestBearerMiddleware_MissingHeader(t *testing.T) {
	svc := &mockService{}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Error("next handler should not have been called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestBearerMiddleware_BadScheme(t *testing.T) {
	svc := &mockService{}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Basic dXNlcjpwYXNz")

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Error("next handler should not have been called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestBearerMiddleware_InvalidToken(t *testing.T) {
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			return nil, tokens.ErrInvalidToken
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer badtoken")

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Error("next handler should not have been called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestBearerMiddleware_ExpiredToken(t *testing.T) {
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			return nil, tokens.ErrExpiredToken
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer expiredtoken")

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Error("next handler should not have been called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
	// Check that the code is auth.expired_token, not auth.invalid_token
	body := w.Body.String()
	if body == "" {
		t.Fatal("expected non-empty body")
	}
	// The body should contain "expired_token"
	if !contains(body, "expired_token") {
		t.Errorf("expected 'expired_token' in body, got: %s", body)
	}
}

func TestBearerMiddleware_RevokedToken(t *testing.T) {
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			return nil, tokens.ErrRevokedToken
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer revokedtoken")

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Error("next handler should not have been called")
	}
	if w.Code != http.StatusUnauthorized {
		t.Errorf("want 401, got %d", w.Code)
	}
}

func TestBearerMiddleware_ValidToken_AttachesAccount(t *testing.T) {
	want := &store.Account{ID: "acct-001", Email: "test@example.com"}
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			return want, nil
		},
	}
	mw := tokens.BearerMiddleware(svc)

	var gotAcct *store.Account
	var inCtx bool
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotAcct, inCtx = tokens.AccountFromContext(r.Context())
		w.WriteHeader(http.StatusOK)
	})

	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	r.Header.Set("Authorization", "Bearer validtoken")

	mw(handler).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !inCtx {
		t.Error("account not found in context")
	}
	if gotAcct == nil || gotAcct.ID != want.ID {
		t.Errorf("wrong account in context: got %v, want %v", gotAcct, want)
	}
}

func TestAccountFromContext_Missing(t *testing.T) {
	ctx := context.Background()
	acct, ok := tokens.AccountFromContext(ctx)
	if ok {
		t.Error("want ok=false when no account in context")
	}
	if acct != nil {
		t.Error("want nil account when not in context")
	}
}

// contains is a simple substring check for test bodies.
func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && containsStr(s, sub))
}

func containsStr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
