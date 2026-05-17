package accounts_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/accounts"
	portalauth "jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// Test double
// ---------------------------------------------------------------------------

// fakeClock is a controllable time source used to exercise the
// accounts.Handler's CreateOrg / CreateOrgInvite / AcceptOrgInvite
// clock-injection paths. Mirrors the shape of internal/portal/auth's
// test-only fakeClock; a local copy keeps the test package independent.
type fakeClock struct {
	t time.Time
}

func (f *fakeClock) Now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

// ---------------------------------------------------------------------------
// Env wired with NewWithClock
// ---------------------------------------------------------------------------

// newOrgsClockTestEnv mirrors newOrgsMembersTestEnv but wires the
// accounts.Handler via NewWithClock so the fakeClock is in scope.
type orgsClockTestEnv struct {
	srv   *httptest.Server
	svc   tokens.Service
	s     store.Store
	clock *fakeClock
}

func newOrgsClockTestEnv(t *testing.T, clk *fakeClock) *orgsClockTestEnv {
	t.Helper()
	s := openStore(t)
	svc := tokens.New(s)
	h := accounts.NewWithClock(s, &noopSender{}, "https://portal.example.com", clk)

	strictHandler := openapi.NewStrictHandlerWithOptions(&accountsOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})

	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictHandler,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Post("/api/orgs", apiWrapper.CreateOrg)

		r.Group(func(r chi.Router) {
			r.Use(portalauth.RequireOrgRole(s, "creator"))
			r.Post("/api/orgs/{orgID}/invites", apiWrapper.CreateOrgInvite)
		})

		r.Post("/api/orgs/{orgID}/invites/{inviteID}/accept", apiWrapper.AcceptOrgInvite)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)

	return &orgsClockTestEnv{srv: srv, svc: svc, s: s, clock: clk}
}

func (e *orgsClockTestEnv) bearerToken(t *testing.T, accountID string) string {
	t.Helper()
	pair, err := e.svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("issue token: %v", err)
	}
	return pair.AccessToken
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestCreateOrg_UsesInjectedClock asserts that the org's CreatedAt is
// stamped from the injected fakeClock, not from the real wall clock.
func TestCreateOrg_UsesInjectedClock(t *testing.T) {
	frozen := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: frozen}
	env := newOrgsClockTestEnv(t, clk)

	creator := seedAccount(t, env.s, "clock-creator@example.com")
	tok := env.bearerToken(t, creator.ID)

	resp := postJSON(t, env.srv, "/api/orgs", tok, map[string]any{"name": "ClockOrg"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	orgID, _ := body["id"].(string)
	if orgID == "" {
		t.Fatalf("missing org id in response")
	}

	org, err := env.s.GetOrgByID(context.Background(), orgID)
	if err != nil {
		t.Fatalf("get org: %v", err)
	}
	if !org.CreatedAt.Equal(frozen) {
		t.Errorf("org CreatedAt: want %v, got %v", frozen, org.CreatedAt)
	}

	// Member CreatedAt should also be frozen — same clock instant is used
	// for both inserts in CreateOrg.
	m, err := env.s.GetOrgMember(context.Background(), store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: creator.ID,
	})
	if err != nil {
		t.Fatalf("get org member: %v", err)
	}
	if !m.CreatedAt.Equal(frozen) {
		t.Errorf("member CreatedAt: want %v, got %v", frozen, m.CreatedAt)
	}
}

// TestCreateOrgInvite_UsesInjectedClock asserts that the invite's
// CreatedAt and ExpiresAt are derived from the injected fakeClock —
// ExpiresAt = clock.Now() + 7 days.
func TestCreateOrgInvite_UsesInjectedClock(t *testing.T) {
	frozen := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: frozen}
	env := newOrgsClockTestEnv(t, clk)

	creator := seedAccount(t, env.s, "clock-boss@example.com")
	org := seedOrg(t, env.s, "ClockBossOrg", "clockbossorg")
	seedMember(t, env.s, org.ID, creator.ID, "creator")

	tok := env.bearerToken(t, creator.ID)
	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/invites", tok,
		map[string]any{"email": "clock-invitee@example.com"})
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode: %v", err)
	}
	expiresStr, _ := body["expires_at"].(string)
	if expiresStr == "" {
		t.Fatalf("missing expires_at in response")
	}
	expires, err := time.Parse(time.RFC3339Nano, expiresStr)
	if err != nil {
		t.Fatalf("parse expires_at: %v", err)
	}

	wantExpires := frozen.Add(7 * 24 * time.Hour)
	if !expires.Equal(wantExpires) {
		t.Errorf("ExpiresAt: want %v, got %v", wantExpires, expires)
	}
}

// TestAcceptOrgInvite_ClockPastExpiry_Returns401 advances the fakeClock
// past the invite's ExpiresAt and asserts the handler returns 401
// auth.invalid_token instead of accepting. The DB row's ExpiresAt is
// fixed (seeded ~1 minute ahead); advancing the handler's clock past
// that wall time must produce the expired branch.
func TestAcceptOrgInvite_ClockPastExpiry_Returns401(t *testing.T) {
	frozen := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &fakeClock{t: frozen}
	env := newOrgsClockTestEnv(t, clk)

	inviter := seedAccount(t, env.s, "clock-inv@example.com")
	invitee := seedAccount(t, env.s, "clock-invitee2@example.com")
	org := seedOrg(t, env.s, "ClockExpOrg", "clockexporg")
	seedMember(t, env.s, org.ID, inviter.ID, "creator")

	rawToken := "clockexpiredtoken234567890abcdef1234567890abcdef1234567890abcdef"
	// Seed an invite whose ExpiresAt is exactly 1 minute after `frozen`.
	inv := seedInvite(t, env.s, org.ID, inviter.ID, invitee.Email,
		rawToken, frozen.Add(1*time.Minute))

	// Advance the clock past the invite's ExpiresAt.
	clk.advance(2 * time.Minute)

	tok := env.bearerToken(t, invitee.ID)
	url := "/api/orgs/" + org.ID + "/invites/" + inv.ID + "/accept"
	resp := postJSON(t, env.srv, url, tok, map[string]any{"token": rawToken})
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("want 401, got %d", resp.StatusCode)
	}

	body := map[string]any{}
	json.NewDecoder(resp.Body).Decode(&body)
	if code, _ := body["error"].(string); code != "auth.invalid_token" {
		t.Errorf("error code: want auth.invalid_token, got %q", code)
	}
}
