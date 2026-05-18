package finalize_test

import (
	"context"
	"errors"
	"fmt"
	"net/url"
	"strings"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/tokens"
)

// fetchTokenEnv wires a finalize.Handler with an injected fake-clock token
// service so tests can advance time and assert TTL behaviour.
type fetchTokenEnv struct {
	store    store.Store
	tokenSvc tokens.Service
	clock    *fakeClock
	handler  *finalize.Handler

	orgID  string
	sessID string
	caller store.Account
	other  store.Account

	callerCtx context.Context
	otherCtx  context.Context

	portalURL string
}

// fakeClock is a controllable Now() source for tokens.Service tests.
type fakeClock struct{ t time.Time }

func (f *fakeClock) Now() time.Time          { return f.t }
func (f *fakeClock) advance(d time.Duration) { f.t = f.t.Add(d) }

// newFetchTokenEnv builds a fresh env with the given portalURL plus an
// injected fake clock. The clock is shared between the tokens.Service and
// the test so tests can advance time without races.
func newFetchTokenEnv(t *testing.T, portalURL string) *fetchTokenEnv {
	t.Helper()

	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	clk := &fakeClock{t: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)}
	tokSvc := tokens.NewWithClock(s, clk)
	log := events.New(s)
	handler := finalize.New(s, &stubStorage{}, log, tokSvc, portalURL)

	orgID := ulid.Make().String()
	callerID := ulid.Make().String()
	otherID := ulid.Make().String()
	sessID := ulid.Make().String()

	now := clk.Now()
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "ftorg", Slug: fmt.Sprintf("ftorg-%s", orgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	caller, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: callerID, Email: fmt.Sprintf("caller-%s@ex.com", callerID[:8]),
		DisplayName: "Caller", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create caller: %v", err)
	}
	otherAcc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: otherID, Email: fmt.Sprintf("other-%s@ex.com", otherID[:8]),
		DisplayName: "Other", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create other: %v", err)
	}

	// Caller is org+session member; other is org-only (no session member).
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: callerID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add caller org: %v", err)
	}
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: orgID, AccountID: otherID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add other org: %v", err)
	}

	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: orgID, Name: "ftest", Goal: "test",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "finalizing", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: orgID, SessionID: sessID, AccountID: callerID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add caller session: %v", err)
	}

	return &fetchTokenEnv{
		store:     s,
		tokenSvc:  tokSvc,
		clock:     clk,
		handler:   handler,
		orgID:     orgID,
		sessID:    sessID,
		caller:    caller,
		other:     otherAcc,
		callerCtx: tokens.ContextWithAccount(ctx, &caller),
		otherCtx:  tokens.ContextWithAccount(ctx, &otherAcc),
		portalURL: portalURL,
	}
}

func TestIssueFetchToken_HappyPath_TokenValidatesImmediately(t *testing.T) {
	env := newFetchTokenEnv(t, "https://portal.example.com")

	resp, err := env.handler.IssueFetchToken(env.callerCtx, openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r, ok := resp.(openapi.IssueFetchToken201JSONResponse)
	if !ok {
		t.Fatalf("expected 201, got %T", resp)
	}
	if r.Token == "" {
		t.Error("token is empty")
	}
	if r.RemoteUrl == "" {
		t.Error("remote_url is empty")
	}
	if r.ExpiresAt.IsZero() {
		t.Error("expires_at is zero")
	}

	// Token validates immediately against the same service.
	acc, err := env.tokenSvc.Validate(context.Background(), r.Token)
	if err != nil {
		t.Fatalf("Validate immediate: %v", err)
	}
	if acc.ID != env.caller.ID {
		t.Errorf("token bound to wrong account: got %s want %s", acc.ID, env.caller.ID)
	}
}

func TestIssueFetchToken_ExpiresAfter5Minutes(t *testing.T) {
	env := newFetchTokenEnv(t, "https://portal.example.com")

	resp, err := env.handler.IssueFetchToken(env.callerCtx, openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r := resp.(openapi.IssueFetchToken201JSONResponse)

	// expires_at is exactly 5 minutes after the clock's "now" at issuance.
	wantExpiry := env.clock.Now().Add(5 * time.Minute)
	if !r.ExpiresAt.Equal(wantExpiry) {
		t.Errorf("expires_at = %s, want %s", r.ExpiresAt, wantExpiry)
	}

	// Just before expiry: still valid.
	env.clock.advance(5*time.Minute - time.Second)
	if _, err := env.tokenSvc.Validate(context.Background(), r.Token); err != nil {
		t.Errorf("Validate just-before-expiry: %v", err)
	}

	// Past expiry: ErrExpiredToken.
	env.clock.advance(2 * time.Second)
	_, err = env.tokenSvc.Validate(context.Background(), r.Token)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("Validate post-expiry: want ErrExpiredToken, got %v", err)
	}
}

func TestIssueFetchToken_RemoteURLCarriesTokenInUserinfo_HTTPS(t *testing.T) {
	env := newFetchTokenEnv(t, "https://portal.example.com")

	resp, err := env.handler.IssueFetchToken(env.callerCtx, openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r := resp.(openapi.IssueFetchToken201JSONResponse)

	u, err := url.Parse(r.RemoteUrl)
	if err != nil {
		t.Fatalf("parse remote_url %q: %v", r.RemoteUrl, err)
	}
	if u.Scheme != "https" {
		t.Errorf("scheme = %s, want https", u.Scheme)
	}
	if u.Host != "portal.example.com" {
		t.Errorf("host = %s, want portal.example.com", u.Host)
	}
	if u.User == nil {
		t.Fatal("userinfo missing from remote_url")
	}
	if u.User.Username() != "x-access-token" {
		t.Errorf("userinfo username = %s, want x-access-token", u.User.Username())
	}
	pw, set := u.User.Password()
	if !set || pw != r.Token {
		t.Errorf("userinfo password = (%q,%v), want (%q,true)", pw, set, r.Token)
	}
	wantPath := fmt.Sprintf("/git/%s/%s.git", env.orgID, env.sessID)
	if u.Path != wantPath {
		t.Errorf("path = %s, want %s", u.Path, wantPath)
	}
}

func TestIssueFetchToken_RemoteURLCarriesTokenInUserinfo_HTTPLocalhost(t *testing.T) {
	env := newFetchTokenEnv(t, "http://localhost:8080")

	resp, err := env.handler.IssueFetchToken(env.callerCtx, openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	r := resp.(openapi.IssueFetchToken201JSONResponse)

	u, err := url.Parse(r.RemoteUrl)
	if err != nil {
		t.Fatalf("parse remote_url %q: %v", r.RemoteUrl, err)
	}
	if u.Scheme != "http" {
		t.Errorf("scheme = %s, want http", u.Scheme)
	}
	if u.Host != "localhost:8080" {
		t.Errorf("host = %s, want localhost:8080", u.Host)
	}
	if u.User == nil || u.User.Username() != "x-access-token" {
		t.Errorf("userinfo missing or wrong username: %v", u.User)
	}
	// Sanity: serialized URL begins with http://x-access-token: pattern.
	if !strings.HasPrefix(r.RemoteUrl, "http://x-access-token:") {
		t.Errorf("remote_url = %q, want http://x-access-token: prefix", r.RemoteUrl)
	}
}

func TestIssueFetchToken_NonMember_403(t *testing.T) {
	env := newFetchTokenEnv(t, "https://portal.example.com")

	resp, err := env.handler.IssueFetchToken(env.otherCtx, openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, ok := resp.(openapi.IssueFetchToken403JSONResponse); !ok {
		t.Fatalf("expected 403, got %T", resp)
	}
}

func TestIssueFetchToken_Unauthenticated_401(t *testing.T) {
	env := newFetchTokenEnv(t, "https://portal.example.com")
	resp, err := env.handler.IssueFetchToken(context.Background(), openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: env.sessID,
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, ok := resp.(openapi.IssueFetchToken401JSONResponse); !ok {
		t.Fatalf("expected 401, got %T", resp)
	}
}

func TestIssueFetchToken_SessionNotFound_404(t *testing.T) {
	env := newFetchTokenEnv(t, "https://portal.example.com")
	resp, err := env.handler.IssueFetchToken(env.callerCtx, openapi.IssueFetchTokenRequestObject{
		OrgID:     env.orgID,
		SessionID: ulid.Make().String(),
	})
	if err != nil {
		t.Fatalf("issue: %v", err)
	}
	if _, ok := resp.(openapi.IssueFetchToken404JSONResponse); !ok {
		t.Fatalf("expected 404, got %T", resp)
	}
}
