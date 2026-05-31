package sessionresume_test

import (
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/sessionresume"
	"jamsesh/internal/portal/tokens"
)

// ---------------------------------------------------------------------------
// exchangeTestEnv — extends testEnv with exchange-specific setup
// ---------------------------------------------------------------------------

// exchangeEnv is a test environment wired for exchange tests. It reuses the
// shared db.Open / fakeClock pattern from the mint tests and adds:
//   - a minted resume token (consumed or pending)
//   - an anonymous account (playground path)
//   - a durable account (durable path)
type exchangeEnv struct {
	s     store.Store
	rawDB *sql.DB // underlying *sql.DB for direct account-row counting
	clock *fakeClock
	handler *sessionresume.Handler
	tokSvc  tokens.Service

	// playground session
	pgOrgID  string
	pgSessID string
	anonAcct store.Account
	// anonBearerRaw is the raw bearer issued at playground-join time, used
	// to verify no new account was created on exchange.
	anonBearerRaw string

	// durable session
	durOrgID  string
	durSessID string
	durAcct   store.Account
}

// newExchangeEnv builds a minimal in-memory SQLite environment for exchange tests.
func newExchangeEnv(t *testing.T) *exchangeEnv {
	t.Helper()
	ctx := context.Background()

	s, rawDB, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	clk := &fakeClock{t: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)}
	tokSvc := tokens.NewWithClock(s, clk)
	handler := sessionresume.NewWithClock(s, tokSvc, "https://portal.example.com", clk)

	now := clk.Now()
	const pgOrgID = "org_playground"
	pgSessID := ulid.Make().String()
	durOrgID := ulid.Make().String()
	durSessID := ulid.Make().String()

	// ---- Playground org + session ----
	// Order: org → session → bearer (FK: session must exist before bearer).
	if _, err := s.CreateProtectedOrg(ctx, store.CreateProtectedOrgParams{
		ID: pgOrgID, Name: "playground", Slug: "playground", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create playground org: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: pgSessID, OrgID: pgOrgID, Name: "pg-sess", Goal: "test",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: now,
		HardCapAt: timePtr(now.Add(4 * time.Hour)),
	}); err != nil {
		t.Fatalf("create pg session: %v", err)
	}
	// Anonymous account — simulates a user who created/joined a playground session.
	// Must be called AFTER the session row exists (bearer has a session_id FK).
	anonRawBearer, anonID, _, err := tokSvc.IssueAnonymousSessionBearer(ctx, pgSessID, "amber-otter", 4*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}
	anonAcct, err := s.GetAccountByID(ctx, anonID)
	if err != nil {
		t.Fatalf("GetAccountByID (anon): %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: pgOrgID, SessionID: pgSessID, AccountID: anonID, Role: "member", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add anon session member: %v", err)
	}

	// ---- Durable org + session ----
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: durOrgID, Name: "durorg", Slug: fmt.Sprintf("dur-%s", durOrgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create durable org: %v", err)
	}
	durAcct, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          "dur-" + ulid.Make().String(),
		Email:       fmt.Sprintf("user-%s@ex.com", durOrgID[:8]),
		DisplayName: "DurableUser",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("create durable account: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: durSessID, OrgID: durOrgID, Name: "dur-sess", Goal: "test",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create durable session: %v", err)
	}
	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: durOrgID, AccountID: durAcct.ID, Role: "creator", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add durable org member: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: durOrgID, SessionID: durSessID, AccountID: durAcct.ID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add durable session member: %v", err)
	}

	return &exchangeEnv{
		s:             s,
		rawDB:         rawDB,
		clock:         clk,
		handler:       handler,
		tokSvc:        tokSvc,
		pgOrgID:       pgOrgID,
		pgSessID:      pgSessID,
		anonAcct:      anonAcct,
		anonBearerRaw: anonRawBearer,
		durOrgID:      durOrgID,
		durSessID:     durSessID,
		durAcct:       durAcct,
	}
}

// mintResumeToken mints a resume token bound to (orgID, sessionID, accountID)
// and returns its raw value. Fails the test on any error.
func (e *exchangeEnv) mintResumeToken(t *testing.T, orgID, sessionID, accountID string) string {
	t.Helper()
	ctx := context.Background()
	acct, err := e.s.GetAccountByID(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID(%s): %v", accountID, err)
	}
	callerCtx := tokens.ContextWithAccount(ctx, &acct)
	resp, err := e.handler.CreateSessionResume(callerCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId:     orgID,
			SessionId: sessionID,
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionResume: %v", err)
	}
	r, ok := resp.(openapi.CreateSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("CreateSessionResume returned %T, want 200", resp)
	}
	// Extract raw token from the URL fragment (#rt=<token>).
	resumeURL := r.ResumeUrl
	for i, c := range resumeURL {
		if c == '#' {
			return resumeURL[i+4:] // skip "#rt="
		}
	}
	t.Fatalf("no #rt= fragment in resume_url %q", resumeURL)
	return ""
}

// countAccounts returns the total number of rows in the accounts table.
// It uses a direct SQL query against the underlying *sql.DB so that a stray
// new anonymous account created during exchange would always be caught —
// regardless of whether the new account happens to be an org member of any
// of the test orgs.
func (e *exchangeEnv) countAccounts(t *testing.T) int {
	t.Helper()
	var n int
	if err := e.rawDB.QueryRowContext(context.Background(), `SELECT COUNT(*) FROM accounts`).Scan(&n); err != nil {
		t.Fatalf("countAccounts: %v", err)
	}
	return n
}

func timePtr(t time.Time) *time.Time { return &t }

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestExchangeSessionResume_ExpiredToken_GenericFailure verifies that an
// expired token returns the same generic 401 as an unknown token.
func TestExchangeSessionResume_ExpiredToken_GenericFailure(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	rawToken := env.mintResumeToken(t, env.pgOrgID, env.pgSessID, env.anonAcct.ID)

	// Advance past the 60-second resume token TTL.
	env.clock.t = env.clock.t.Add(61 * time.Second)

	resp, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{
			ResumeToken: rawToken,
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.ExchangeSessionResume401JSONResponse)
	if !ok {
		t.Fatalf("expired token: expected 401, got %T", resp)
	}
	if r.Error != "auth.invalid_token" {
		t.Errorf("expired token error code = %q, want auth.invalid_token", r.Error)
	}
}

// TestExchangeSessionResume_AlreadyUsedToken_GenericFailure verifies that an
// already-consumed token returns the same generic 401.
func TestExchangeSessionResume_AlreadyUsedToken_GenericFailure(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	rawToken := env.mintResumeToken(t, env.pgOrgID, env.pgSessID, env.anonAcct.ID)

	// First exchange — should succeed.
	resp1, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawToken},
	})
	if err != nil {
		t.Fatalf("first exchange error: %v", err)
	}
	if _, ok := resp1.(openapi.ExchangeSessionResume200JSONResponse); !ok {
		t.Fatalf("first exchange: expected 200, got %T", resp1)
	}

	// Second exchange of the SAME token — must fail generically.
	resp2, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawToken},
	})
	if err != nil {
		t.Fatalf("second exchange error: %v", err)
	}
	r, ok := resp2.(openapi.ExchangeSessionResume401JSONResponse)
	if !ok {
		t.Fatalf("second exchange (already-used): expected 401, got %T", resp2)
	}
	if r.Error != "auth.invalid_token" {
		t.Errorf("already-used error code = %q, want auth.invalid_token", r.Error)
	}
}

// TestExchangeSessionResume_UnknownToken_GenericFailure verifies that a
// token that was never minted returns the generic 401 — same shape as
// expired and already-used (no oracle).
func TestExchangeSessionResume_UnknownToken_GenericFailure(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	resp, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{
			ResumeToken: "deadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeefdeadbeef",
		},
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	r, ok := resp.(openapi.ExchangeSessionResume401JSONResponse)
	if !ok {
		t.Fatalf("unknown token: expected 401, got %T", resp)
	}
	if r.Error != "auth.invalid_token" {
		t.Errorf("unknown token error code = %q, want auth.invalid_token", r.Error)
	}
}

// TestExchangeSessionResume_GenericFailure_SameErrorShape asserts the
// no-oracle property: all three failure cases return the SAME error code and
// message so a caller cannot distinguish them.
func TestExchangeSessionResume_GenericFailure_SameErrorShape(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	// Case 1: unknown token.
	respUnknown, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{
			ResumeToken: "aaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaaa1",
		},
	})
	if err != nil {
		t.Fatalf("unknown: %v", err)
	}

	// Case 2: expired token.
	rawExpired := env.mintResumeToken(t, env.pgOrgID, env.pgSessID, env.anonAcct.ID)
	env.clock.t = env.clock.t.Add(61 * time.Second)
	respExpired, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawExpired},
	})
	if err != nil {
		t.Fatalf("expired: %v", err)
	}
	// Rewind clock for the used-token case.
	env.clock.t = env.clock.t.Add(-61 * time.Second)

	// Case 3: already-used token.
	rawUsed := env.mintResumeToken(t, env.durOrgID, env.durSessID, env.durAcct.ID)
	if _, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawUsed},
	}); err != nil {
		t.Fatalf("first durable exchange: %v", err)
	}
	respUsed, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawUsed},
	})
	if err != nil {
		t.Fatalf("used: %v", err)
	}

	extractErr := func(t *testing.T, resp openapi.ExchangeSessionResumeResponseObject, label string) (code, msg string) {
		t.Helper()
		r, ok := resp.(openapi.ExchangeSessionResume401JSONResponse)
		if !ok {
			t.Fatalf("%s: expected 401JSONResponse, got %T", label, resp)
		}
		return r.Error, r.Message
	}

	codeUnknown, msgUnknown := extractErr(t, respUnknown, "unknown")
	codeExpired, msgExpired := extractErr(t, respExpired, "expired")
	codeUsed, msgUsed := extractErr(t, respUsed, "used")

	if codeUnknown != codeExpired || codeExpired != codeUsed {
		t.Errorf("error codes differ: unknown=%q expired=%q used=%q (no-oracle violated)",
			codeUnknown, codeExpired, codeUsed)
	}
	if msgUnknown != msgExpired || msgExpired != msgUsed {
		t.Errorf("messages differ: unknown=%q expired=%q used=%q (no-oracle violated)",
			msgUnknown, msgExpired, msgUsed)
	}
}

// TestExchangeSessionResume_SingleUseUnderConcurrency verifies that exactly
// one of N parallel exchanges of the same token wins (gets a bearer) and the
// rest get the generic 401. The single-use property is guaranteed by the
// atomic winner-returning ConsumeResumeToken (UPDATE…RETURNING WHERE
// used_at IS NULL): exactly one goroutine's UPDATE matches the row; all others
// see zero rows → ErrNotFound.
//
// A file-backed SQLite DB with MaxOpenConns=8 is used so goroutines genuinely
// race on separate connections. The _txlock=immediate DSN parameter
// (injected by db.Open) gives each BEGIN IMMEDIATE an upfront write-lock,
// preventing SQLITE_BUSY deadlocks while preserving the single-winner property.
func TestExchangeSessionResume_SingleUseUnderConcurrency(t *testing.T) {
	ctx := context.Background()

	// File-backed DB with multiple connections so goroutines race for real.
	dbPath := filepath.Join(t.TempDir(), "concurrent_exchange.db")
	s, _, err := db.Open(ctx, "sqlite", dbPath, db.PoolConfig{MaxOpenConns: 8})
	if err != nil {
		t.Fatalf("open file-backed sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	clk := &fakeClock{t: time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)}
	tokSvc := tokens.NewWithClock(s, clk)
	handler := sessionresume.NewWithClock(s, tokSvc, "https://portal.example.com", clk)

	now := clk.Now()
	const pgOrgID = "org_playground"
	pgSessID := ulid.Make().String()

	// Seed: playground org → session → anon account → session member.
	if _, err := s.CreateProtectedOrg(ctx, store.CreateProtectedOrgParams{
		ID: pgOrgID, Name: "playground", Slug: "playground", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create playground org: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: pgSessID, OrgID: pgOrgID, Name: "pg-conc", Goal: "conc",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active",
		CreatedAt: now, HardCapAt: timePtr(now.Add(4 * time.Hour)),
	}); err != nil {
		t.Fatalf("create pg session: %v", err)
	}
	_, anonID, _, err := tokSvc.IssueAnonymousSessionBearer(ctx, pgSessID, "conc-otter", 4*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: pgOrgID, SessionID: pgSessID, AccountID: anonID, Role: "member", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add anon session member: %v", err)
	}

	// Mint a single resume token — the prize all goroutines race for.
	anonAcct, err := s.GetAccountByID(ctx, anonID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	anonCtx := tokens.ContextWithAccount(ctx, &anonAcct)
	mintResp, err := handler.CreateSessionResume(anonCtx, openapi.CreateSessionResumeRequestObject{
		Body: &openapi.CreateSessionResumeJSONRequestBody{
			OrgId: pgOrgID, SessionId: pgSessID,
		},
	})
	if err != nil {
		t.Fatalf("CreateSessionResume: %v", err)
	}
	mr, ok := mintResp.(openapi.CreateSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("mint returned %T, want 200", mintResp)
	}
	resumeURL := mr.ResumeUrl
	var rawToken string
	for i, c := range resumeURL {
		if c == '#' {
			rawToken = resumeURL[i+4:]
			break
		}
	}
	if rawToken == "" {
		t.Fatalf("no #rt= fragment in resume_url %q", resumeURL)
	}

	const N = 8
	type result struct {
		resp openapi.ExchangeSessionResumeResponseObject
		err  error
	}
	results := make([]result, N)
	var wg sync.WaitGroup
	wg.Add(N)

	// startGun ensures all goroutines begin exchanging simultaneously.
	var startGun sync.WaitGroup
	startGun.Add(1)
	ready := make(chan struct{})

	for i := 0; i < N; i++ {
		i := i
		go func() {
			defer wg.Done()
			// Signal ready and wait for the start gun.
			ready <- struct{}{}
			startGun.Wait()

			resp, err := handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
				Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawToken},
			})
			results[i] = result{resp: resp, err: err}
		}()
	}

	// Wait until all goroutines are ready, then release them together.
	for i := 0; i < N; i++ {
		<-ready
	}
	startGun.Done()
	wg.Wait()

	// Tally winners (200) and losers (401).
	winners := 0
	losers := 0
	for i, r := range results {
		if r.err != nil {
			t.Errorf("goroutine %d: unexpected error: %v", i, r.err)
			continue
		}
		switch resp := r.resp.(type) {
		case openapi.ExchangeSessionResume200JSONResponse:
			winners++
			if resp.AccountId != anonID {
				t.Errorf("goroutine %d winner: account_id = %q, want %q", i, resp.AccountId, anonID)
			}
		case openapi.ExchangeSessionResume401JSONResponse:
			losers++
			if resp.Error != "auth.invalid_token" {
				t.Errorf("goroutine %d loser: error = %q, want auth.invalid_token", i, resp.Error)
			}
		default:
			t.Errorf("goroutine %d: unexpected response type %T", i, r.resp)
		}
	}

	if winners != 1 {
		t.Errorf("concurrent exchange: %d winners, want exactly 1 (single-use violated)", winners)
	}
	if losers != N-1 {
		t.Errorf("concurrent exchange: %d losers, want %d", losers, N-1)
	}
}

// TestExchangeSessionResume_Playground_NoNewAccount verifies that exchanging a
// playground resume token:
//   - issues a bearer whose account_id == the original anonymous account ID
//   - does NOT create any new account rows
//   - the issued bearer validates as a session member (exercises handlerauth)
func TestExchangeSessionResume_Playground_NoNewAccount(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	// Baseline account count BEFORE the exchange.
	baselineCount := env.countAccounts(t)

	rawToken := env.mintResumeToken(t, env.pgOrgID, env.pgSessID, env.anonAcct.ID)

	resp, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawToken},
	})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	r, ok := resp.(openapi.ExchangeSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("playground exchange: expected 200, got %T", resp)
	}

	// ---- No new account was created ----
	afterCount := env.countAccounts(t)
	if afterCount != baselineCount {
		t.Errorf("account count changed: before=%d after=%d (exchange must not create new accounts)",
			baselineCount, afterCount)
	}

	// ---- account_id matches the minting bearer's account ----
	if r.AccountId != env.anonAcct.ID {
		t.Errorf("account_id = %q, want %q", r.AccountId, env.anonAcct.ID)
	}

	// ---- Kind == playground ----
	if r.Kind != openapi.Playground {
		t.Errorf("kind = %q, want playground", r.Kind)
	}

	// ---- session_id and org_id are correct ----
	if r.SessionId != env.pgSessID {
		t.Errorf("session_id = %q, want %q", r.SessionId, env.pgSessID)
	}
	if r.OrgId != env.pgOrgID {
		t.Errorf("org_id = %q, want %q", r.OrgId, env.pgOrgID)
	}

	// ---- The issued bearer validates and is accepted as a session member ----
	issuedAcct, err := env.tokSvc.Validate(ctx, r.Bearer)
	if err != nil {
		t.Fatalf("issued bearer Validate: %v", err)
	}
	if issuedAcct.ID != env.anonAcct.ID {
		t.Errorf("Validate account_id = %q, want %q", issuedAcct.ID, env.anonAcct.ID)
	}
	if !issuedAcct.IsAnonymous {
		t.Error("issued bearer account IsAnonymous = false, want true")
	}

	// Exercise handlerauth.RequireSessionMember — the same check all
	// session-scoped handlers use to gate playground access.
	authCtx := tokens.ContextWithAccount(ctx, issuedAcct)
	_, _, fail, ok := handlerauth.RequireSessionMember(authCtx, env.s, env.pgOrgID, env.pgSessID)
	if !ok {
		t.Errorf("issued bearer should be accepted as session member; fail=%+v", fail)
	}
}

// TestExchangeSessionResume_Durable_IssuedNoRefreshToken verifies that a
// durable exchange returns a short-lived access token with kind=durable and
// no refresh token in the response.
func TestExchangeSessionResume_Durable_IssuedNoRefreshToken(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	rawToken := env.mintResumeToken(t, env.durOrgID, env.durSessID, env.durAcct.ID)

	resp, err := env.handler.ExchangeSessionResume(ctx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawToken},
	})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	r, ok := resp.(openapi.ExchangeSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("durable exchange: expected 200, got %T", resp)
	}

	if r.Kind != openapi.Durable {
		t.Errorf("kind = %q, want durable", r.Kind)
	}
	if r.AccountId != env.durAcct.ID {
		t.Errorf("account_id = %q, want %q", r.AccountId, env.durAcct.ID)
	}
	if r.SessionId != env.durSessID {
		t.Errorf("session_id = %q, want %q", r.SessionId, env.durSessID)
	}
	if r.OrgId != env.durOrgID {
		t.Errorf("org_id = %q, want %q", r.OrgId, env.durOrgID)
	}

	// Validate the access token — must succeed.
	issuedAcct, err := env.tokSvc.Validate(ctx, r.Bearer)
	if err != nil {
		t.Fatalf("Validate durable bearer: %v", err)
	}
	if issuedAcct.ID != env.durAcct.ID {
		t.Errorf("Validate account = %q, want %q", issuedAcct.ID, env.durAcct.ID)
	}
	if issuedAcct.IsAnonymous {
		t.Error("durable account IsAnonymous = true, want false")
	}

	// No refresh token in the response — the SessionResumeExchangeResponse
	// struct has only Bearer + ExpiresAt, so there is no place for a refresh
	// token; the assertion is structural.
	// Confirm the issued token row has kind="access" (not "refresh").
	bearerHash := sha256HexOfStr(r.Bearer)
	row, err := env.s.GetOAuthTokenByHash(ctx, bearerHash)
	if err != nil {
		t.Fatalf("GetOAuthTokenByHash: %v", err)
	}
	if row.Kind == "refresh" {
		t.Error("durable exchange issued a refresh token — short-lived credential must not include refresh")
	}
	if row.Kind != "access" {
		t.Errorf("token kind = %q, want access", row.Kind)
	}
}

// TestExchangeSessionResume_AmbientAuthHeaderIgnored verifies that sending
// an Authorization: Bearer header (with an unrelated valid token) does not
// affect exchange — the resume_token is the sole credential.
func TestExchangeSessionResume_AmbientAuthHeaderIgnored(t *testing.T) {
	env := newExchangeEnv(t)
	ctx := context.Background()

	// Mint a resume token for the playground session.
	rawToken := env.mintResumeToken(t, env.pgOrgID, env.pgSessID, env.anonAcct.ID)

	// Place an "ambient" account into context — as if BearerMiddleware had run.
	// Exchange must ignore this entirely (it doesn't call AccountFromContext).
	ambientCtx := tokens.ContextWithAccount(ctx, &env.durAcct) // wrong account!

	resp, err := env.handler.ExchangeSessionResume(ambientCtx, openapi.ExchangeSessionResumeRequestObject{
		Body: &openapi.ExchangeSessionResumeJSONRequestBody{ResumeToken: rawToken},
	})
	if err != nil {
		t.Fatalf("exchange: %v", err)
	}
	r, ok := resp.(openapi.ExchangeSessionResume200JSONResponse)
	if !ok {
		t.Fatalf("ambient-auth exchange: expected 200, got %T", resp)
	}

	// Result must be scoped to the token's bound account (the anon account),
	// NOT the ambient durable account from the context.
	if r.AccountId != env.anonAcct.ID {
		t.Errorf("account_id = %q, want anon %q (ambient ctx must be ignored)",
			r.AccountId, env.anonAcct.ID)
	}
}

// openStoreWithSessionLocal opens a fresh in-memory SQLite store and creates
// a minimal org + session row for tests that need session_id FK. Returns the
// store and the session ID. Duplicates the helper from the tokens_test package
// since test helpers are package-private.
func openStoreWithSessionLocal(t *testing.T) (store.Store, string) {
	t.Helper()
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-exchange-test",
		Name:      "Exchange Test Org",
		Slug:      "exchange-test-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-exchange-001",
		OrgID:         org.ID,
		Name:          "Exchange Session",
		Goal:          "test exchange",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return s, sess.ID
}

// TestIssueAnonymousSessionBearerForExistingAccount_RejectsNonAnonymousAccount
// verifies that the new tokens method returns ErrForbidden when a durable
// (non-anonymous) account ID is supplied.
func TestIssueAnonymousSessionBearerForExistingAccount_RejectsNonAnonymousAccount(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSessionLocal(t)
	svc := tokens.New(s)

	// Create a normal (durable) account.
	durable, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          "dur-" + ulid.Make().String(),
		Email:       "durable@example.com",
		DisplayName: "Durable User",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	_, _, err = svc.IssueAnonymousSessionBearerForExistingAccount(ctx, durable.ID, sessID, time.Hour)
	if err == nil {
		t.Fatal("expected ErrForbidden for non-anonymous account, got nil")
	}
	if !isForbiddenErr(err) {
		t.Errorf("want ErrForbidden, got %v", err)
	}
}

// TestIssueAnonymousSessionBearerForExistingAccount_SucceedsForAnonAccount
// verifies that the method mints a bearer for an existing anonymous account
// WITHOUT creating a new account row.
func TestIssueAnonymousSessionBearerForExistingAccount_SucceedsForAnonAccount(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSessionLocal(t)
	svc := tokens.New(s)

	// Create an anonymous account (as IssueAnonymousSessionBearer would).
	_, anonID, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "test-otter", time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	rawBearer, expiresAt, err := svc.IssueAnonymousSessionBearerForExistingAccount(ctx, anonID, sessID, time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearerForExistingAccount: %v", err)
	}
	if len(rawBearer) != 64 {
		t.Errorf("rawBearer length = %d, want 64", len(rawBearer))
	}
	if expiresAt.IsZero() {
		t.Error("expiresAt is zero")
	}

	// The issued bearer must validate and point to the ORIGINAL account.
	acct, err := svc.Validate(ctx, rawBearer)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if acct.ID != anonID {
		t.Errorf("account_id = %q, want %q", acct.ID, anonID)
	}
	if !acct.IsAnonymous {
		t.Error("IsAnonymous = false, want true")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// sha256HexOfStr returns the hex-encoded SHA-256 digest of s.
func sha256HexOfStr(s string) string {
	sum := sha256.Sum256([]byte(s))
	return hex.EncodeToString(sum[:])
}

// isForbiddenErr returns true when err wraps or equals tokens.ErrForbidden.
func isForbiddenErr(err error) bool {
	return errors.Is(err, tokens.ErrForbidden)
}
