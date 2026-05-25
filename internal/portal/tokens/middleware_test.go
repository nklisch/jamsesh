package tokens_test

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

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

func (m *mockService) IssueShortLived(_ context.Context, _ string, _ time.Duration) (string, time.Time, error) {
	return "", time.Time{}, errors.New("not implemented")
}

func (m *mockService) IssueAnonymousSessionBearer(_ context.Context, _, _ string, _ time.Duration) (string, string, time.Time, error) {
	return "", "", time.Time{}, errors.New("not implemented")
}

func (m *mockService) Validate(ctx context.Context, raw string) (*store.Account, error) {
	return m.validateFn(ctx, raw)
}

func (m *mockService) Refresh(_ context.Context, _ string) (tokens.Pair, error) {
	return tokens.Pair{}, errors.New("not implemented")
}

func (m *mockService) Revoke(_ context.Context, _ string, _ string, _ bool) error {
	return errors.New("not implemented")
}

func (m *mockService) RevokeAnonymousBearer(_ context.Context, _ string) error {
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

// TestBearerMiddleware_TransientDBError_TypedEnvelope asserts that when
// svc.Validate fails with a non-sentinel error (the canonical case being
// a transient DB outage — pgx connection failure, etc.), the middleware
// emits the typed `dep.db_unavailable` envelope (503 + Retry-After: 2)
// rather than the legacy generic `internal` 500.
//
// Regression guard for story portal-bearer-middleware-dep-translate.
func TestBearerMiddleware_TransientDBError_TypedEnvelope(t *testing.T) {
	transient := errors.New("unexpected EOF")
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			return nil, transient
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer some-valid-looking-token")

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Fatal("next handler should not have been called when validate fails")
	}
	if w.Code != http.StatusServiceUnavailable {
		t.Errorf("want 503, got %d", w.Code)
	}
	if got := w.Header().Get("Retry-After"); got != "2" {
		t.Errorf("want Retry-After=2, got %q", got)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/json; charset=utf-8" {
		t.Errorf("want application/json content-type, got %q", ct)
	}
	var env struct {
		Error   string `json:"error"`
		Message string `json:"message"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, w.Body.String())
	}
	if env.Error != "dep.db_unavailable" {
		t.Errorf("want error=dep.db_unavailable, got %q\nbody: %s", env.Error, w.Body.String())
	}
	if env.Message == "" {
		t.Errorf("want non-empty message, body: %s", w.Body.String())
	}
}

// TestBearerMiddleware_BusinessSentinel_PassesThrough asserts that the
// dep wrap helper does not misclassify business sentinels routed through
// the middleware's default branch. store.ErrNotFound from svc.Validate
// is unexpected (the token-sentinel cases catch the real not-found
// paths) but should still fall through to ErrInternal — not be wrapped
// as dep.db_unavailable — because WrapDBIfTransient passes the sentinel
// through unchanged. This guards against a future change that might
// over-broaden the dep wrap.
func TestBearerMiddleware_BusinessSentinel_PassesThrough(t *testing.T) {
	svc := &mockService{
		validateFn: func(_ context.Context, _ string) (*store.Account, error) {
			return nil, store.ErrNotFound
		},
	}
	mw := tokens.BearerMiddleware(svc)

	reached := false
	w := httptest.NewRecorder()
	r := httptest.NewRequest(http.MethodGet, "/api/me", nil)
	r.Header.Set("Authorization", "Bearer some-token")

	mw(nextHandler(&reached)).ServeHTTP(w, r)

	if reached {
		t.Fatal("next handler should not have been called")
	}
	if w.Code != http.StatusInternalServerError {
		t.Errorf("want 500 (business sentinel falls through to ErrInternal), got %d", w.Code)
	}
	var env struct {
		Error string `json:"error"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &env); err != nil {
		t.Fatalf("decode envelope: %v\nbody: %s", err, w.Body.String())
	}
	if env.Error != "internal" {
		t.Errorf("want error=internal, got %q\nbody: %s", env.Error, w.Body.String())
	}
}

// TestBearerMiddleware_AnonymousBearer_AuthenticatesRequest is a regression test
// that confirms BearerMiddleware accepts an anonymous session bearer and injects
// the anonymous account (IsAnonymous: true) into context. This guards against
// future refactors that might accidentally branch on identity kind in the
// Bearer middleware path.
func TestBearerMiddleware_AnonymousBearer_AuthenticatesRequest(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)

	// Create org + session for the anonymous bearer's session_id FK.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-mw-anon",
		Name:      "MW Anon Org",
		Slug:      "mw-anon-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-mw-anon",
		OrgID:         org.ID,
		Name:          "MW Anon Session",
		Goal:          "test anon bearer in middleware",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	svc := tokens.New(s)
	rawToken, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sess.ID, "jade-jackal", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
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
	r.Header.Set("Authorization", "Bearer "+rawToken)

	mw(handler).ServeHTTP(w, r)

	if w.Code != http.StatusOK {
		t.Errorf("want 200, got %d", w.Code)
	}
	if !inCtx {
		t.Error("account not found in context")
	}
	if gotAcct == nil || gotAcct.ID != accountID {
		t.Errorf("wrong account in context: got %v, want ID %q", gotAcct, accountID)
	}
	if gotAcct != nil && !gotAcct.IsAnonymous {
		t.Error("IsAnonymous should be true for anonymous bearer account in context")
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
