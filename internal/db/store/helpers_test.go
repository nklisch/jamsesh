// Package store_test provides fixture helpers for the cross-dialect store
// test suite. The dialect harness itself lives in
// jamsesh/internal/db/store/storetest so it is importable from other _test
// packages (this _test package cannot itself be imported).
//
// # Postgres path
//
// When JAMSESH_TEST_PG_DSN is set it must point at a throwaway database that
// the test user owns. Tests will TRUNCATE all tables with CASCADE between runs
// but will NOT drop or recreate the schema — run migrations once before the
// suite (db.Open does this automatically on first connection).
//
// The recommended pattern for CI is to spin up a fresh Postgres container per
// pipeline and point JAMSESH_TEST_PG_DSN at it. Local developer iteration uses
// SQLite only (no env var required).
package store_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/db/store/storetest"
)

// stores is a one-line wrapper over storetest.Stores so existing call sites
// in this package don't have to spell the package-qualified name each time.
// The dialect-harness type is the exported storetest.DialectHarness.
func stores(t *testing.T) []storetest.DialectHarness {
	t.Helper()
	return storetest.Stores(t)
}

// ---------------------------------------------------------------------------
// must* fixture helpers — all call t.Helper() and t.Fatal on failure.
// ---------------------------------------------------------------------------

// nextID is a simple counter for generating unique IDs in tests. Not
// thread-safe, but tests within a single goroutine are sequential.
var nextIDCounter int

func nextID(prefix string) string {
	nextIDCounter++
	return fmt.Sprintf("%s-%04d", prefix, nextIDCounter)
}

func mustCreateOrg(t *testing.T, ctx context.Context, s store.Store, slug string) store.Org {
	t.Helper()
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        nextID("org"),
		Name:      "Org " + slug,
		Slug:      slug,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustCreateOrg(%q): %v", slug, err)
	}
	return org
}

func mustCreateAccount(t *testing.T, ctx context.Context, s store.Store, email string) store.Account {
	t.Helper()
	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:           nextID("acc"),
		Email:        email,
		DisplayName:  email,
		GithubUserID: nil,
		CreatedAt:    time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustCreateAccount(%q): %v", email, err)
	}
	return acc
}

func mustAddOrgMember(t *testing.T, ctx context.Context, s store.Store, orgID, accountID, role string) {
	t.Helper()
	err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID:     orgID,
		AccountID: accountID,
		Role:      role,
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustAddOrgMember(org=%q, acc=%q): %v", orgID, accountID, err)
	}
}

func mustCreateSession(t *testing.T, ctx context.Context, s store.Store, orgID, name string) store.Session {
	t.Helper()
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            nextID("sess"),
		OrgID:         orgID,
		Name:          name,
		Goal:          "goal for " + name,
		WritableScope: `["src/"]`,
		DefaultMode:   "sync",
		BaseSHA:       nil,
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustCreateSession(org=%q, name=%q): %v", orgID, name, err)
	}
	return sess
}

func mustAddSessionMember(t *testing.T, ctx context.Context, s store.Store, orgID, sessionID, accountID, role string) {
	t.Helper()
	err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: accountID,
		Role:      role,
		JoinedAt:  time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustAddSessionMember(org=%q, sess=%q, acc=%q): %v", orgID, sessionID, accountID, err)
	}
}

// assertNoError fails the test if err is non-nil.
func assertNoError(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
}

// assertOnlyContains asserts that list contains exactly the given session IDs
// (order-insensitive) and no others.
func assertOnlyContains(t *testing.T, list []store.Session, want ...string) {
	t.Helper()
	got := make(map[string]bool, len(list))
	for _, s := range list {
		got[s.ID] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, id := range want {
		wantSet[id] = true
	}
	for id := range got {
		if !wantSet[id] {
			t.Errorf("list contains unexpected session %q", id)
		}
	}
	for id := range wantSet {
		if !got[id] {
			t.Errorf("list missing expected session %q", id)
		}
	}
}

// assertSessionMemberIDs asserts that the member list contains exactly the
// given accountIDs (order-insensitive) and no others.
func assertSessionMemberIDs(t *testing.T, list []store.SessionMember, want ...string) {
	t.Helper()
	got := make(map[string]bool, len(list))
	for _, m := range list {
		got[m.AccountID] = true
	}
	wantSet := make(map[string]bool, len(want))
	for _, id := range want {
		wantSet[id] = true
	}
	for id := range got {
		if !wantSet[id] {
			t.Errorf("member list contains unexpected account %q", id)
		}
	}
	for id := range wantSet {
		if !got[id] {
			t.Errorf("member list missing expected account %q", id)
		}
	}
}

