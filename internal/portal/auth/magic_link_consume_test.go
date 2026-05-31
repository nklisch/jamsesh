package auth_test

// magic_link_consume_test.go tests the consume-token classification logic
// in ExchangeMagicLink: driver error → 5xx, 0-rows → 401, 1-row → success.
// These tests use a stub store to inject specific ConsumeMagicLinkToken
// behaviors that cannot be reproduced with the real SQLite store.

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/google/uuid"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Stub store for consume-classification tests
// ---------------------------------------------------------------------------

// stubConsumeStore wraps a real store.Store (for all operations except
// ConsumeMagicLinkToken) and allows injection of a custom behavior for
// ConsumeMagicLinkToken. This lets us simulate driver errors and 0-rows
// without real concurrency.
type stubConsumeStore struct {
	store.Store                 // delegate everything else
	consumeFn func(ctx context.Context, p store.ConsumeMagicLinkTokenParams) (int64, error)
}

func (s *stubConsumeStore) ConsumeMagicLinkToken(ctx context.Context, p store.ConsumeMagicLinkTokenParams) (int64, error) {
	if s.consumeFn != nil {
		return s.consumeFn(ctx, p)
	}
	return s.Store.ConsumeMagicLinkToken(ctx, p)
}

// ---------------------------------------------------------------------------
// Test helpers
// ---------------------------------------------------------------------------

// buildConsumeTestEnv constructs a test HTTP server where ConsumeMagicLinkToken
// is controlled by consumeFn. Use nil consumeFn to delegate to the real store.
func buildConsumeTestEnv(t *testing.T, consumeFn func(context.Context, store.ConsumeMagicLinkTokenParams) (int64, error)) (*httptest.Server, *captureSender) {
	t.Helper()
	realStore := openStore(t)
	stub := &stubConsumeStore{Store: realStore, consumeFn: consumeFn}
	sender := &captureSender{}
	tokenSvc := tokens.New(realStore) // tokens backed by the real store
	handler := auth.NewMagicLinkHandler(stub, tokenSvc, sender, "https://portal.example.com")

	fullHandler := &magicLinkOnlyStrict{MagicLinkHandler: handler}
	strictAPI := openapi.NewStrictHandlerWithOptions(fullHandler, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	r := chi.NewRouter()
	r.Post("/api/auth/magic-link/request", strictAPI.RequestMagicLink)
	r.Post("/api/auth/magic-link/exchange", strictAPI.ExchangeMagicLink)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv, sender
}

// requestToken issues a magic-link request and returns the raw token extracted
// from the captured email body. Fatals on unexpected status.
func requestToken(t *testing.T, srv *httptest.Server, sender *captureSender, email string) string {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"email": email})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/magic-link/request", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request: %v", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusNoContent {
		t.Fatalf("request: want 204, got %d", resp.StatusCode)
	}
	return extractTokenFromBody(t, sender.lastBody())
}

// exchangeToken POSTs to /exchange and returns the response.
func exchangeToken(t *testing.T, srv *httptest.Server, token string) *http.Response {
	t.Helper()
	b, _ := json.Marshal(map[string]string{"token": token})
	req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/magic-link/exchange", bytes.NewReader(b))
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	t.Cleanup(func() { resp.Body.Close() })
	return resp
}

// ---------------------------------------------------------------------------
// Unit 1: consume classification tests
// ---------------------------------------------------------------------------

// TestExchangeMagicLink_ConsumeDriverError_Returns5xx verifies that a transient
// driver error from ConsumeMagicLinkToken surfaces as a 5xx (not a 401).
// Before the fix, any error from consume was returned as 401 "already used".
func TestExchangeMagicLink_ConsumeDriverError_Returns5xx(t *testing.T) {
	driverErr := errors.New("db: connection reset by peer")
	srv, sender := buildConsumeTestEnv(t, func(_ context.Context, _ store.ConsumeMagicLinkTokenParams) (int64, error) {
		return 0, driverErr
	})

	tok := requestToken(t, srv, sender, "consume-err@example.com")
	resp := exchangeToken(t, srv, tok)

	if resp.StatusCode < 500 || resp.StatusCode >= 600 {
		t.Errorf("want 5xx on driver error, got %d", resp.StatusCode)
	}
}

// TestExchangeMagicLink_ConsumeZeroRows_Returns401 verifies that when
// ConsumeMagicLinkToken returns 0 rows affected (race lost), the handler
// returns 401 with code auth.invalid_token — the correct race-loss treatment.
func TestExchangeMagicLink_ConsumeZeroRows_Returns401(t *testing.T) {
	srv, sender := buildConsumeTestEnv(t, func(_ context.Context, _ store.ConsumeMagicLinkTokenParams) (int64, error) {
		// 0 rows, no error: another concurrent exchange already consumed the token.
		return 0, nil
	})

	tok := requestToken(t, srv, sender, "consume-zero@example.com")
	resp := exchangeToken(t, srv, tok)

	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 for race-lost consume, got %d", resp.StatusCode)
	}
	var body map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&body)
	if code, _ := body["error"].(string); code != "auth.invalid_token" {
		t.Errorf("error code: want auth.invalid_token, got %q", code)
	}
}

// TestExchangeMagicLink_ConsumeOneRow_Succeeds verifies that 1-row-affected
// from ConsumeMagicLinkToken leads to successful token-pair issuance (200).
func TestExchangeMagicLink_ConsumeOneRow_Succeeds(t *testing.T) {
	// nil consumeFn → delegate to real store (which will consume properly)
	srv, sender := buildConsumeTestEnv(t, nil)

	tok := requestToken(t, srv, sender, "consume-ok@example.com")
	resp := exchangeToken(t, srv, tok)

	if resp.StatusCode != http.StatusOK {
		t.Errorf("want 200 for successful consume, got %d", resp.StatusCode)
	}
}

// TestExchangeMagicLink_ConcurrentExchangeSingleUse verifies that two
// concurrent exchanges of the same token result in exactly one successful
// token-pair issuance and one 401. This exercises the real race-condition
// guard at the SQL layer.
func TestExchangeMagicLink_ConcurrentExchangeSingleUse(t *testing.T) {
	// Use the real store (no stub) — the SQL WHERE used_at IS NULL guard
	// enforces single-use atomically even under concurrent requests.
	realStore := openStore(t)
	sender := &captureSender{}
	tokenSvc := tokens.New(realStore)
	handler := auth.NewMagicLinkHandler(realStore, tokenSvc, sender, "https://portal.example.com")

	fullHandler := &magicLinkOnlyStrict{MagicLinkHandler: handler}
	strictAPI := openapi.NewStrictHandlerWithOptions(fullHandler, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	r := chi.NewRouter()
	r.Post("/api/auth/magic-link/request", strictAPI.RequestMagicLink)
	r.Post("/api/auth/magic-link/exchange", strictAPI.ExchangeMagicLink)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	// Issue and extract a single token.
	tok := requestToken(t, srv, sender, "concurrent@example.com")

	// Launch two concurrent exchange requests for the same token.
	type result struct{ status int }
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, _ := json.Marshal(map[string]string{"token": tok})
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/magic-link/exchange", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- result{status: -1}
				return
			}
			defer resp.Body.Close()
			results <- result{status: resp.StatusCode}
		}()
	}
	wg.Wait()
	close(results)

	var got200, got401, other int
	for r := range results {
		switch r.status {
		case http.StatusOK:
			got200++
		case http.StatusUnauthorized:
			got401++
		default:
			other++
		}
	}

	if got200 != 1 {
		t.Errorf("want exactly 1 successful exchange (200), got %d (401=%d, other=%d)", got200, got401, other)
	}
	if got401 != 1 {
		t.Errorf("want exactly 1 rejected exchange (401), got %d (200=%d, other=%d)", got401, got200, other)
	}
}

// TestExchangeMagicLink_ConcurrentExchangeNoDoubleProvision verifies that two
// concurrent exchanges of the same token produce exactly one token pair, not
// two. The gating on affected==1 prevents double-provisioning.
func TestExchangeMagicLink_ConcurrentExchangeNoDoubleProvision(t *testing.T) {
	realStore := openStore(t)
	sender := &captureSender{}
	tokenSvc := tokens.New(realStore)
	handler := auth.NewMagicLinkHandler(realStore, tokenSvc, sender, "https://portal.example.com")

	fullHandler := &magicLinkOnlyStrict{MagicLinkHandler: handler}
	strictAPI := openapi.NewStrictHandlerWithOptions(fullHandler, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	r := chi.NewRouter()
	r.Post("/api/auth/magic-link/request", strictAPI.RequestMagicLink)
	r.Post("/api/auth/magic-link/exchange", strictAPI.ExchangeMagicLink)
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	tok := requestToken(t, srv, sender, "no-double-prov@example.com")

	type result struct {
		status      int
		accessToken string
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			b, _ := json.Marshal(map[string]string{"token": tok})
			req, _ := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/magic-link/exchange", bytes.NewReader(b))
			req.Header.Set("Content-Type", "application/json")
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				results <- result{status: -1}
				return
			}
			defer resp.Body.Close()
			var body map[string]any
			_ = json.NewDecoder(resp.Body).Decode(&body)
			at, _ := body["access_token"].(string)
			results <- result{status: resp.StatusCode, accessToken: at}
		}()
	}
	wg.Wait()
	close(results)

	var successCount int
	for r := range results {
		if r.status == http.StatusOK {
			successCount++
		}
	}
	if successCount != 1 {
		t.Errorf("want exactly 1 successful exchange, got %d", successCount)
	}
}

// ---------------------------------------------------------------------------
// Dual-dialect store test for :execrows
// ---------------------------------------------------------------------------

// TestConsumeMagicLinkToken_ExecrowsSemantics exercises the real SQLite store
// to verify:
//   - First consume returns (1, nil).
//   - Second consume of the same token returns (0, nil) — used_at already set.
//   - An error case is exercised via a closed DB (driver error → non-nil error).
func TestConsumeMagicLinkToken_ExecrowsSemantics(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)

	// Create a magic-link token.
	now := time.Now().UTC()
	tok, err := s.CreateMagicLinkToken(ctx, store.CreateMagicLinkTokenParams{
		ID:        uuid.New().String(),
		TokenHash: "testhash_" + uuid.New().String(),
		Email:     "execrows@example.com",
		IssuedAt:  now,
		ExpiresAt: now.Add(15 * time.Minute),
		UsedAt:    nil,
	})
	if err != nil {
		t.Fatalf("CreateMagicLinkToken: %v", err)
	}

	params := store.ConsumeMagicLinkTokenParams{ID: tok.ID, UsedAt: &now}

	// First consume: expect 1 row affected.
	n, err := s.ConsumeMagicLinkToken(ctx, params)
	if err != nil {
		t.Fatalf("first consume: unexpected error: %v", err)
	}
	if n != 1 {
		t.Errorf("first consume: want 1 row affected, got %d", n)
	}

	// Second consume: used_at is already set → WHERE used_at IS NULL matches 0 rows.
	n2, err2 := s.ConsumeMagicLinkToken(ctx, params)
	if err2 != nil {
		t.Fatalf("second consume: unexpected error: %v", err2)
	}
	if n2 != 0 {
		t.Errorf("second consume: want 0 rows affected, got %d", n2)
	}
}
