package finalize_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// stubStorage is the minimal storage.Service stub used by lock tests.
// The lock-CRUD endpoints in this story do not exercise repo paths.
type stubStorage struct{}

func (s *stubStorage) RepoPath(orgID, sessionID string) string {
	return "/tmp/" + orgID + "/" + sessionID
}
func (s *stubStorage) CreateRepo(_ context.Context, _, _ string) error  { return nil }
func (s *stubStorage) RemoveRepo(_ context.Context, _, _ string) error  { return nil }
func (s *stubStorage) RepoExists(_, _ string) (bool, error)             { return false, nil }
func (s *stubStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (s *stubStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, store.ErrNotFound
}
func (s *stubStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

// finalizeEnv holds the wired pieces tests reach into.
type finalizeEnv struct {
	store   store.Store
	log     *events.Log
	handler *finalize.Handler

	orgID   string
	sessID  string
	caller  store.Account
	otherID string

	// callerCtx is a context carrying the caller account on the
	// tokens.AccountFromContext key — the handler reads identity from
	// this directly without going through HTTP middleware.
	callerCtx context.Context
	otherCtx  context.Context
}

func newFinalizeEnv(t *testing.T) *finalizeEnv {
	return newFinalizeEnvPool(t, db.PoolConfig{})
}

// newFinalizeEnvPool is like newFinalizeEnv but accepts an explicit PoolConfig.
// Use PoolConfig{MaxOpenConns: 1} for concurrency tests so that all goroutines
// share the single in-memory SQLite connection and see the same schema/data.
func newFinalizeEnvPool(t *testing.T, pc db.PoolConfig) *finalizeEnv {
	t.Helper()

	ctx := context.Background()
	s, _, err := db.Open(ctx, "sqlite", ":memory:", pc)
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	log := events.New(s)
	tokSvc := tokens.New(s)
	handler := finalize.New(s, &stubStorage{}, log, tokSvc, "https://portal.test")

	now := time.Now().UTC()
	orgID := ulid.Make().String()
	callerID := ulid.Make().String()
	otherID := ulid.Make().String()
	sessID := ulid.Make().String()

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: "testorg", Slug: fmt.Sprintf("testorg-%s", orgID[:8]), CreatedAt: now,
	}); err != nil {
		t.Fatalf("create org: %v", err)
	}
	caller, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: callerID, Email: fmt.Sprintf("caller-%s@example.com", callerID[:8]),
		DisplayName: "Caller", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create caller account: %v", err)
	}
	if _, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID: otherID, Email: fmt.Sprintf("other-%s@example.com", otherID[:8]),
		DisplayName: "Other", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create other account: %v", err)
	}

	for _, accID := range []string{callerID, otherID} {
		if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
			OrgID: orgID, AccountID: accID, Role: "member", CreatedAt: now,
		}); err != nil {
			t.Fatalf("add org member %s: %v", accID, err)
		}
	}

	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: orgID, Name: "finalize-test", Goal: "test goal",
		WritableScope: `["**"]`, DefaultMode: "sync", Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create session: %v", err)
	}

	for _, accID := range []string{callerID, otherID} {
		role := "member"
		if accID == callerID {
			role = "creator"
		}
		if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
			OrgID: orgID, SessionID: sessID, AccountID: accID, Role: role, JoinedAt: now,
		}); err != nil {
			t.Fatalf("add session member %s: %v", accID, err)
		}
	}

	other, _ := s.GetAccountByID(ctx, otherID)

	return &finalizeEnv{
		store:     s,
		log:       log,
		handler:   handler,
		orgID:     orgID,
		sessID:    sessID,
		caller:    caller,
		otherID:   otherID,
		callerCtx: tokens.ContextWithAccount(ctx, &caller),
		otherCtx:  tokens.ContextWithAccount(ctx, &other),
	}
}

// contextWithAccount is a test-local alias for tokens.ContextWithAccount.
func contextWithAccount(ctx context.Context, acct *store.Account) context.Context {
	return tokens.ContextWithAccount(ctx, acct)
}

// ensure the finalize package import is materially used so go test doesn't
// complain about unused imports when only behaviour through env.handler is
// exercised.
var _ = finalize.FinalizeLockTTL

// testFinalizeStore mirrors the unexported finalizeStore interface so
// newFinalizeHandlerWith can accept narrow mock types without requiring them
// to implement the full store.Store umbrella.
type testFinalizeStore interface {
	store.FinalizeLockStore
	store.SessionStore
	store.SessionMemberStore
	store.OrgMemberStore
	store.AccountStore
}

// newFinalizeHandlerWith builds a finalize.Handler backed by the supplied
// narrow store. baseStore is used for support services (events.Log,
// tokens.Service) that require the full store interface; this is typically
// the same real store the narrow mock wraps.
func newFinalizeHandlerWith(t *testing.T, s testFinalizeStore, baseStore store.Store) *finalize.Handler {
	t.Helper()
	log := events.New(baseStore)
	tokSvc := tokens.New(baseStore)
	return finalize.New(s, &stubStorage{}, log, tokSvc, "https://portal.test")
}
