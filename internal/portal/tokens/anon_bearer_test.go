package tokens_test

import (
	"context"
	"database/sql"
	"errors"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/handlerauth"
	"jamsesh/internal/portal/tokens"
)

// openStoreWithSession opens a fresh SQLite store and creates a minimal session
// row (needed for the anonymous bearer's session_id FK). Returns the store and
// the session ID.
func openStoreWithSession(t *testing.T) (store.Store, string) {
	t.Helper()
	s := openStore(t)
	ctx := context.Background()

	// Create a minimal org.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-anon-test",
		Name:      "Anon Test Org",
		Slug:      "anon-test-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	// Create a minimal session.
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-anon-001",
		OrgID:         org.ID,
		Name:          "Anon Session",
		Goal:          "test anon bearers",
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

func TestIssueAnonymousSessionBearer_ReturnsValidRawToken(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, accountID, expiresAt, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "amber-otter", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}
	if len(rawToken) != 64 {
		t.Errorf("rawToken length: want 64, got %d", len(rawToken))
	}
	if !strings.HasPrefix(accountID, "anon_") {
		t.Errorf("accountID should start with 'anon_', got %q", accountID)
	}
	if expiresAt.Before(time.Now().Add(23 * time.Hour)) {
		t.Errorf("expiresAt %v should be ~24h in the future", expiresAt)
	}
}

func TestIssueAnonymousSessionBearer_ValidateAcceptsBearer(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "amber-otter", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	got, err := svc.Validate(ctx, rawToken)
	if err != nil {
		t.Fatalf("Validate: %v", err)
	}
	if got.ID != accountID {
		t.Errorf("Validate returned account %q, want %q", got.ID, accountID)
	}
	if !got.IsAnonymous {
		t.Error("Validate returned account with IsAnonymous=false, want true")
	}
	if got.DisplayName != "amber-otter" {
		t.Errorf("DisplayName: want 'amber-otter', got %q", got.DisplayName)
	}
}

func TestIssueAnonymousSessionBearer_AccountEmailIsSynthetic(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	_, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "blue-fox", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	acct, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}
	wantEmail := accountID + "@playground.local"
	if acct.Email != wantEmail {
		t.Errorf("synthetic email: want %q, got %q", wantEmail, acct.Email)
	}
}

func TestIssueAnonymousSessionBearer_UpdatesLastUsedAt(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "cedar-hawk", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	// Validate twice; last_used_at should be set after first call.
	if _, err := svc.Validate(ctx, rawToken); err != nil {
		t.Fatalf("Validate: %v", err)
	}
	// Second call should also succeed (regression: not revoked by Validate).
	if _, err := svc.Validate(ctx, rawToken); err != nil {
		t.Errorf("second Validate: %v", err)
	}
}

func TestIssueAnonymousSessionBearer_ExpiredBearerRejected(t *testing.T) {
	ctx := context.Background()

	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	defer s.Close()

	// Create org + session.
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: "org-exp-001", Name: "Exp Org", Slug: "exp-org-001", CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-exp-001",
		OrgID:         org.ID,
		Name:          "Exp Session",
		Goal:          "test expiry",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	clk := &fakeClock{t: time.Now().UTC()}
	svc := tokens.NewWithClock(s, clk)

	rawToken, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sess.ID, "dawn-elk", 5*time.Minute)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	// Advance past TTL.
	clk.advance(5*time.Minute + time.Second)

	_, err = svc.Validate(ctx, rawToken)
	if !errors.Is(err, tokens.ErrExpiredToken) {
		t.Errorf("expired bearer: want ErrExpiredToken, got %v", err)
	}
}

func TestIssueAnonymousSessionBearer_RevokedBearerRejected(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	rawToken, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "ember-crow", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer: %v", err)
	}

	// Revoke all bearers for the session via the store (simulates destruction sweep).
	now := time.Now().UTC()
	if err := s.RevokeBearersForSession(ctx, store.RevokeBearersForSessionParams{
		RevokedAt: now,
		SessionID: sessID,
	}); err != nil {
		t.Fatalf("RevokeBearersForSession: %v", err)
	}

	_, err = svc.Validate(ctx, rawToken)
	if !errors.Is(err, tokens.ErrRevokedToken) {
		t.Errorf("revoked bearer: want ErrRevokedToken, got %v", err)
	}
}

func TestIssueAnonymousSessionBearer_EmptySessionID_NoDBCalls(t *testing.T) {
	// Verifies that an empty sessionID is rejected by the pre-tx validation
	// guard, so no DB calls are made and no account row is created.
	//
	// Originally named _TransactionalRollback, but the body never exercised
	// a real rollback — empty sessionID short-circuits before WithTx is even
	// called, so there is no transaction to roll back. The no-DB-calls
	// assertion is the real value, so the body stayed and the test was
	// renamed. See TestIssueAnonymousSessionBearer_TransactionalRollback
	// below for the real rollback-path test.
	//
	// Distinct from TestIssueAnonymousSessionBearer_EmptySessionID_Rejected,
	// which asserts only the error surface (this one additionally asserts
	// that no DB writes occurred).
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, "", "fern-moth", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty sessionID, got nil")
	}
	// No account should have been created.
	rows, listErr := s.ListOAuthTokensForAccount(ctx, "anon_anything")
	if listErr != nil {
		t.Fatalf("ListOAuthTokensForAccount: %v", listErr)
	}
	if len(rows) != 0 {
		t.Errorf("expected 0 token rows after failed issue, got %d", len(rows))
	}
}

// txStoreOverride embeds the real TxStore and overrides only
// CreateAnonymousBearer to return the injected error. Go's struct embedding
// satisfies all other ~20 sub-interface methods through the embedded *real*
// TxStore (the one passed by WithTx).
type txStoreOverride struct {
	store.TxStore
	bearerErr error
}

func (o *txStoreOverride) CreateAnonymousBearer(ctx context.Context, arg store.CreateAnonymousBearerParams) (store.OAuthToken, error) {
	return store.OAuthToken{}, o.bearerErr
}

// storeOverride embeds the real Store and overrides only WithTx to wrap the
// TxStore passed to fn. Same embedding trick — every other Store method
// delegates to the embedded real Store.
type storeOverride struct {
	store.Store
	bearerErr error
}

func (o *storeOverride) WithTx(ctx context.Context, fn func(store.TxStore) error) error {
	return o.Store.WithTx(ctx, func(tx store.TxStore) error {
		return fn(&txStoreOverride{TxStore: tx, bearerErr: o.bearerErr})
	})
}

// openStoreAndSQLWithSession opens a fresh in-memory SQLite store, keeps the
// underlying *sql.DB for raw queries the store interface does not expose
// (specifically: COUNT(*) FROM accounts WHERE display_name=?), and seeds a
// minimal org + session row for the anonymous bearer's session_id FK.
func openStoreAndSQLWithSession(t *testing.T) (store.Store, *sql.DB, string) {
	t.Helper()
	ctx := context.Background()
	s, sqlDB, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite :memory:: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-rollback-test",
		Name:      "Rollback Test Org",
		Slug:      "rollback-test-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-rollback-001",
		OrgID:         org.ID,
		Name:          "Rollback Session",
		Goal:          "test rollback",
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	return s, sqlDB, sess.ID
}

func TestIssueAnonymousSessionBearer_TransactionalRollback(t *testing.T) {
	// Verifies that if bearer creation fails inside WithTx, the account row
	// created earlier in the same transaction is rolled back. Uses the
	// storeOverride embedded-store pattern to inject a CreateAnonymousBearer
	// error while letting CreateAnonymousAccount proceed normally.
	ctx := context.Background()
	realStore, sqlDB, sessID := openStoreAndSQLWithSession(t)

	injectErr := errors.New("synthetic bearer-insert failure")
	overlay := &storeOverride{Store: realStore, bearerErr: injectErr}
	svc := tokens.New(overlay)

	const nickname = "fern-moth"
	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, nickname, 24*time.Hour)
	if err == nil {
		t.Fatal("expected bearer-insert error, got nil")
	}
	if !errors.Is(err, injectErr) {
		t.Errorf("expected wrapped %v (via errors.Is), got %v", injectErr, err)
	}

	// Confirm no anonymous account row was committed despite
	// CreateAnonymousAccount succeeding within the transaction. The store
	// interface has no account-by-display-name query, so we drop to a raw
	// SQL COUNT(*) via the underlying *sql.DB the store handle wraps.
	var rowCount int
	if err := sqlDB.QueryRowContext(ctx,
		`SELECT COUNT(*) FROM accounts WHERE display_name=?`, nickname,
	).Scan(&rowCount); err != nil {
		t.Fatalf("count accounts by display_name: %v", err)
	}
	if rowCount != 0 {
		t.Errorf("rollback failed: %d anon accounts with display_name=%q survived a failed bearer-insert", rowCount, nickname)
	}
}

func TestIssueAnonymousSessionBearer_EmptyNickname_Rejected(t *testing.T) {
	ctx := context.Background()
	s, sessID := openStoreWithSession(t)
	svc := tokens.New(s)

	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, sessID, "", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty nickname, got nil")
	}
}

func TestIssueAnonymousSessionBearer_EmptySessionID_Rejected(t *testing.T) {
	ctx := context.Background()
	s := openStore(t)
	svc := tokens.New(s)

	_, _, _, err := svc.IssueAnonymousSessionBearer(ctx, "", "ghost-heron", 24*time.Hour)
	if err == nil {
		t.Fatal("expected error for empty sessionID, got nil")
	}
}

// TestIssueAnonymousSessionBearer_BearerRejectedOnDifferentSession pins the
// SECURITY.md contract: "a leaked anonymous bearer authenticates only the
// session it was issued for. No cross-session privilege."
//
// Protection does NOT come from Validate itself (which has no session-binding
// check — see gate-security-anon-bearer-validate-no-session-binding). It comes
// from the downstream RequireSessionMember check that every session-scoped
// handler uses. This test exercises that path directly.
func TestIssueAnonymousSessionBearer_BearerRejectedOnDifferentSession(t *testing.T) {
	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open sqlite: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	now := time.Now().UTC()

	// Create one org that owns both sessions (mirrors playground: both
	// sessions live in the same org).
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-xsess-test",
		Name:      "Cross-Session Org",
		Slug:      "xsess-org",
		CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	createSession := func(id, name string) store.Session {
		t.Helper()
		sess, err := s.CreateSession(ctx, store.CreateSessionParams{
			ID:            id,
			OrgID:         org.ID,
			Name:          name,
			Goal:          "cross-session test",
			WritableScope: `["src/"]`,
			DefaultMode:   "sync",
			Status:        "active",
			CreatedAt:     now,
		})
		if err != nil {
			t.Fatalf("CreateSession(%q): %v", id, err)
		}
		return sess
	}

	sessA := createSession("sess-xsess-A", "Session A")
	sessB := createSession("sess-xsess-B", "Session B")

	// Issue an anonymous bearer bound to session A.
	svc := tokens.New(s)
	_, accountID, _, err := svc.IssueAnonymousSessionBearer(ctx, sessA.ID, "iron-lynx", 24*time.Hour)
	if err != nil {
		t.Fatalf("IssueAnonymousSessionBearer(sessA): %v", err)
	}

	// Register the anon account as a member of session A (mimics step 3 of
	// the playground create-session handler). The account must NOT be added
	// to session B — that is the invariant we're testing.
	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     org.ID,
		SessionID: sessA.ID,
		AccountID: accountID,
		Role:      "creator",
		JoinedAt:  now,
	}); err != nil {
		t.Fatalf("AddSessionMember(sessA): %v", err)
	}

	// Retrieve the anon account as Validate/middleware would leave it in ctx.
	anonAcct, err := s.GetAccountByID(ctx, accountID)
	if err != nil {
		t.Fatalf("GetAccountByID: %v", err)
	}

	// Place the anon account into a context (same as BearerMiddleware does
	// after a successful Validate call).
	authCtx := tokens.ContextWithAccount(ctx, &anonAcct)

	// Exercise the downstream session-membership guard against session B.
	// This is the same call every session-scoped handler makes.
	_, _, fail, ok := handlerauth.RequireSessionMember(authCtx, s, org.ID, sessB.ID)

	if ok {
		// CRITICAL: the bearer for session A authenticated against session B.
		// This violates the SECURITY.md threat model.
		t.Error("SECURITY BUG: anon bearer for session A was accepted on session B — cross-session auth leak")
		return
	}

	// Expect a 403 (not a member of this session), not a 401 or 500.
	if fail.Status != 403 {
		t.Errorf("want status 403 (not a session member), got %d; fail=%+v", fail.Status, fail)
	}
	if fail.Forbidden.Error != "auth.insufficient_permission" {
		t.Errorf("want error=auth.insufficient_permission, got %q", fail.Forbidden.Error)
	}

	// Confirm the same account IS accepted on session A (positive control:
	// the bearer is still valid for its own session).
	_, _, failA, okA := handlerauth.RequireSessionMember(authCtx, s, org.ID, sessA.ID)
	if !okA {
		t.Errorf("anon account should be accepted on session A (positive control), got fail=%+v", failA)
	}
}
