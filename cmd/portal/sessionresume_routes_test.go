package main

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"
	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/ratelimit"
	portalrouter "jamsesh/internal/portal/router"
	"jamsesh/internal/portal/sessionresume"
	"jamsesh/internal/portal/tokens"
)

type sessionResumeRouteClock struct{ t time.Time }

func (c sessionResumeRouteClock) Now() time.Time { return c.t }

type sessionResumeRouteHarness struct {
	handler http.Handler
	token   string
	orgID   string
	sessID  string
	resume  *sessionresume.Handler
	account store.Account
}

func newSessionResumeRouteHarness(t *testing.T) *sessionResumeRouteHarness {
	t.Helper()

	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Date(2026, 5, 31, 12, 0, 0, 0, time.UTC)
	orgID := ulid.Make().String()
	accountID := ulid.Make().String()
	sessionID := ulid.Make().String()

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "resume-route", Slug: fmt.Sprintf("resume-%s", orgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	account, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: accountID, Email: fmt.Sprintf("route-%s@example.com", accountID[:8]), DisplayName: "Route User", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create account: %v", err)
	}
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: accountID, Role: "creator", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add org member: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessionID, OrgID: orgID, Name: "route-session", Goal: "test route",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessionID, AccountID: accountID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add session member: %v", err)
	}

	tokenSvc := tokens.New(s)
	pair, err := tokenSvc.Issue(ctx, accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	resumeHandler := sessionresume.NewWithClock(s, tokenSvc, "https://portal.example.test", sessionResumeRouteClock{t: now})

	strict := openapi.NewStrictHandlerWithOptions(&combinedHandler{
		SessionResumeHandler: resumeHandler,
	}, nil, openapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  httperr.WriteBadRequest,
		ResponseErrorHandlerFunc: httperr.WriteFromError,
	})
	wrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strict,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	handler := portalrouter.New(portalrouter.Deps{
		Mounts: portalrouter.Mounts{
			API: func(r chi.Router) {
				sessionResumeRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 10}).Middleware(true)
				r.Group(func(r chi.Router) {
					r.Use(tokens.BearerMiddleware(tokenSvc))
					r.With(sessionResumeRL).Post("/session-resumes", wrapper.CreateSessionResume)
				})

				sessionResumeExchangeRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 10}).Middleware(true)
				r.With(sessionResumeExchangeRL).Post("/session-resumes/exchange", wrapper.ExchangeSessionResume)
			},
		},
	})

	return &sessionResumeRouteHarness{
		handler: handler,
		token:   pair.AccessToken,
		orgID:   orgID,
		sessID:  sessionID,
		resume:  resumeHandler,
		account: account,
	}
}

func (h *sessionResumeRouteHarness) request(method, path, remoteAddr string, body any, bearer string) *httptest.ResponseRecorder {
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.RemoteAddr = remoteAddr
	req.Header.Set("Content-Type", "application/json")
	if bearer != "" {
		req.Header.Set("Authorization", bearer)
	}
	rr := httptest.NewRecorder()
	h.handler.ServeHTTP(rr, req)
	return rr
}

func TestSessionResumeRoutes_MintRequiresBearerAndRateLimits(t *testing.T) {
	h := newSessionResumeRouteHarness(t)
	body := map[string]string{"org_id": h.orgID, "session_id": h.sessID}

	unauth := h.request(http.MethodPost, "/api/session-resumes", "198.51.100.10:1000", body, "")
	if unauth.Code != http.StatusUnauthorized {
		t.Fatalf("unauthenticated mint status = %d, want 401; body=%s", unauth.Code, unauth.Body.String())
	}

	for i := 0; i < 10; i++ {
		rr := h.request(http.MethodPost, "/api/session-resumes", "198.51.100.10:1000", body, "Bearer "+h.token)
		if rr.Code != http.StatusOK {
			t.Fatalf("mint request %d status = %d, want 200; body=%s", i+1, rr.Code, rr.Body.String())
		}
	}

	limited := h.request(http.MethodPost, "/api/session-resumes", "198.51.100.10:1000", body, "Bearer "+h.token)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("mint request over limit status = %d, want 429; body=%s", limited.Code, limited.Body.String())
	}
	if limited.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limited mint response missing Retry-After header")
	}
}

func TestSessionResumeRoutes_ExchangePublicIgnoresAmbientAuthorizationAndRateLimits(t *testing.T) {
	h := newSessionResumeRouteHarness(t)
	rawToken := h.mintRawResumeToken(t)

	ambient := h.request(
		http.MethodPost,
		"/api/session-resumes/exchange",
		"203.0.113.44:1000",
		map[string]string{"resume_token": rawToken},
		"Bearer definitely-invalid",
	)
	if ambient.Code != http.StatusOK {
		t.Fatalf("exchange with ambient invalid Authorization status = %d, want 200; body=%s", ambient.Code, ambient.Body.String())
	}

	for i := 0; i < 10; i++ {
		rr := h.request(
			http.MethodPost,
			"/api/session-resumes/exchange",
			"203.0.113.45:1000",
			map[string]string{"resume_token": fmt.Sprintf("missing-%02d", i)},
			"",
		)
		if rr.Code != http.StatusUnauthorized {
			t.Fatalf("exchange request %d status = %d, want 401 before limit; body=%s", i+1, rr.Code, rr.Body.String())
		}
	}

	limited := h.request(
		http.MethodPost,
		"/api/session-resumes/exchange",
		"203.0.113.45:1000",
		map[string]string{"resume_token": "missing-over-limit"},
		"",
	)
	if limited.Code != http.StatusTooManyRequests {
		t.Fatalf("exchange request over limit status = %d, want 429; body=%s", limited.Code, limited.Body.String())
	}
	if limited.Header().Get("Retry-After") == "" {
		t.Fatalf("rate-limited exchange response missing Retry-After header")
	}
}

func (h *sessionResumeRouteHarness) mintRawResumeToken(t *testing.T) string {
	t.Helper()

	resp, err := h.resume.CreateSessionResume(tokens.ContextWithAccount(context.Background(), &h.account), openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     h.orgID,
			SessionId: h.sessID,
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionResume: %v", err)
	}
	created, ok := resp.(openapi.CreateSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("CreateSessionResume returned %T, want 200", resp)
	}

	u, err := url.Parse(created.ResumeUrl)
	if err != nil {
		t.Fatalf("parse resume_url: %v", err)
	}
	values, err := url.ParseQuery(u.Fragment)
	if err != nil {
		t.Fatalf("parse resume_url fragment: %v", err)
	}
	raw := values.Get("rt")
	if raw == "" {
		t.Fatalf("resume_url fragment %q missing rt token", u.Fragment)
	}
	return raw
}
