package storage_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/storage"
)

// newServiceWithStore creates a Service backed by a fresh in-memory SQLite
// store (migrations applied) and a temp directory for bare repos.
func newServiceWithStore(t *testing.T) (storage.Service, store.Store) {
	t.Helper()
	ctx := context.Background()
	s, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open: %v", err)
	}
	t.Cleanup(func() { s.Close() })

	root := t.TempDir()
	svc := storage.New(root, s)
	return svc, s
}

// seedSession inserts a minimal org + account + session row set that satisfies
// FK constraints and returns the orgID and sessionID used.
func seedSession(t *testing.T, s store.Store) (orgID, sessionID string) {
	t.Helper()
	ctx := context.Background()
	now := time.Now().UTC()

	orgID = "org-test-1"
	sessionID = "sess-test-1"

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        orgID,
		Name:      "Test Org",
		Slug:      "test-org",
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}

	acct, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:          "acct-1",
		Email:       "user@example.com",
		DisplayName: "Test User",
		CreatedAt:   now,
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}

	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessionID,
		OrgID:         orgID,
		Name:          "Test Session",
		Goal:          "Test goal",
		WritableScope: "[]",
		DefaultMode:   "sync",
		Status:        "ended",
		CreatedAt:     now,
		EndedAt:       &now,
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	if err := s.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acct.ID,
		Role:      "creator",
		JoinedAt:  now,
	}); err != nil {
		t.Fatalf("AddSessionMember: %v", err)
	}

	return orgID, sessionID
}

// ---------------------------------------------------------------------------
// ArchiveSession end-to-end
// ---------------------------------------------------------------------------

func TestArchiveSession_EndToEnd(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	svc, s := newServiceWithStore(t)
	orgID, sessionID := seedSession(t, s)

	// Create bare repo.
	if err := svc.CreateRepo(ctx, orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	endedAt := time.Now().UTC().Add(-time.Hour)
	finalBranch := "main"
	info := storage.ArchiveInfo{
		Name:             "Test Session",
		GoalText:         "Test goal",
		MemberAccountIDs: []string{"acct-1"},
		EndedAt:          endedAt,
		EndReason:        "finalize",
		FinalBranchName:  &finalBranch,
	}

	if err := svc.ArchiveSession(ctx, orgID, sessionID, info); err != nil {
		t.Fatalf("ArchiveSession: %v", err)
	}

	// Bare repo must be gone.
	exists, err := svc.RepoExists(orgID, sessionID)
	if err != nil {
		t.Fatalf("RepoExists: %v", err)
	}
	if exists {
		t.Error("bare repo still exists after ArchiveSession")
	}

	// sessions row must be gone.
	if _, err := s.GetSession(ctx, orgID, sessionID); !errors.Is(err, store.ErrNotFound) {
		t.Errorf("GetSession after archive: want ErrNotFound, got %v", err)
	}

	// session_members must be cascade-deleted.
	members, err := s.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})
	if err != nil {
		t.Fatalf("ListSessionMembers: %v", err)
	}
	if len(members) != 0 {
		t.Errorf("session_members not cleaned up: got %d rows", len(members))
	}

	// archived_sessions row must exist.
	rec, err := svc.LookupArchived(ctx, orgID, sessionID)
	if err != nil {
		t.Fatalf("LookupArchived after archive: %v", err)
	}
	if rec.EndReason != "finalize" {
		t.Errorf("EndReason = %q; want %q", rec.EndReason, "finalize")
	}
	if rec.FinalBranchName == nil || *rec.FinalBranchName != "main" {
		t.Errorf("FinalBranchName = %v; want \"main\"", rec.FinalBranchName)
	}
	if len(rec.MemberAccountIDs) != 1 || rec.MemberAccountIDs[0] != "acct-1" {
		t.Errorf("MemberAccountIDs = %v; want [\"acct-1\"]", rec.MemberAccountIDs)
	}
}

// ---------------------------------------------------------------------------
// Re-archive is a no-op
// ---------------------------------------------------------------------------

func TestArchiveSession_RearchiveIsNoop(t *testing.T) {
	requireGit(t)
	ctx := context.Background()
	svc, s := newServiceWithStore(t)
	orgID, sessionID := seedSession(t, s)

	if err := svc.CreateRepo(ctx, orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	endedAt := time.Now().UTC().Add(-time.Hour)
	info := storage.ArchiveInfo{
		Name:             "Test Session",
		GoalText:         "Test goal",
		MemberAccountIDs: []string{"acct-1"},
		EndedAt:          endedAt,
		EndReason:        "abandon",
	}

	// First archive.
	if err := svc.ArchiveSession(ctx, orgID, sessionID, info); err != nil {
		t.Fatalf("first ArchiveSession: %v", err)
	}

	// Second archive — must be no-op (returns nil, row unchanged).
	if err := svc.ArchiveSession(ctx, orgID, sessionID, info); err != nil {
		t.Fatalf("second ArchiveSession (re-archive): %v", err)
	}

	// Row still readable.
	rec, err := svc.LookupArchived(ctx, orgID, sessionID)
	if err != nil {
		t.Fatalf("LookupArchived after re-archive: %v", err)
	}
	if rec.EndReason != "abandon" {
		t.Errorf("EndReason changed after re-archive: got %q", rec.EndReason)
	}
}

// ---------------------------------------------------------------------------
// LookupArchived
// ---------------------------------------------------------------------------

func TestLookupArchived_NotFound(t *testing.T) {
	ctx := context.Background()
	svc, _ := newServiceWithStore(t)

	_, err := svc.LookupArchived(ctx, "no-org", "no-session")
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("LookupArchived on unknown session: want ErrNotFound, got %v", err)
	}
}

func TestLookupArchived_LiveSessionNotFound(t *testing.T) {
	ctx := context.Background()
	svc, s := newServiceWithStore(t)
	orgID, sessionID := seedSession(t, s)

	// Session exists in sessions table but has not been archived.
	_, err := svc.LookupArchived(ctx, orgID, sessionID)
	if !errors.Is(err, store.ErrNotFound) {
		t.Errorf("LookupArchived on live session: want ErrNotFound, got %v", err)
	}
}

// ---------------------------------------------------------------------------
// StubResponse table tests
// ---------------------------------------------------------------------------

func TestStubResponse_WithFinalBranch(t *testing.T) {
	svc, _ := newServiceWithStore(t)

	branch := "release/v1"
	archivedAt := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	rec := &storage.ArchivedRecord{
		SessionID:       "s1",
		OrgID:           "o1",
		EndReason:       "finalize",
		ArchivedAt:      archivedAt,
		FinalBranchName: &branch,
	}

	stub := svc.StubResponse(rec)

	if stub.Error != "session.archived" {
		t.Errorf("Error = %q; want \"session.archived\"", stub.Error)
	}
	if stub.HTTPStatus != 410 {
		t.Errorf("HTTPStatus = %d; want 410", stub.HTTPStatus)
	}
	if stub.Details.EndReason != "finalize" {
		t.Errorf("Details.EndReason = %q; want \"finalize\"", stub.Details.EndReason)
	}
	if stub.Details.FinalBranchName == nil || *stub.Details.FinalBranchName != "release/v1" {
		t.Errorf("Details.FinalBranchName = %v; want \"release/v1\"", stub.Details.FinalBranchName)
	}
	if stub.Details.ArchivedAt == "" {
		t.Error("Details.ArchivedAt is empty")
	}
	// Message should mention the date and branch.
	wantDateStr := "2026-03-15"
	if !containsStr(stub.Message, wantDateStr) {
		t.Errorf("Message = %q; want it to contain %q", stub.Message, wantDateStr)
	}
	if !containsStr(stub.Message, "release/v1") {
		t.Errorf("Message = %q; want it to contain \"release/v1\"", stub.Message)
	}
}

func TestStubResponse_WithoutFinalBranch(t *testing.T) {
	svc, _ := newServiceWithStore(t)

	archivedAt := time.Date(2026, 3, 15, 12, 0, 0, 0, time.UTC)
	rec := &storage.ArchivedRecord{
		SessionID:       "s2",
		OrgID:           "o1",
		EndReason:       "timeout",
		ArchivedAt:      archivedAt,
		FinalBranchName: nil,
	}

	stub := svc.StubResponse(rec)

	if stub.Error != "session.archived" {
		t.Errorf("Error = %q; want \"session.archived\"", stub.Error)
	}
	if stub.HTTPStatus != 410 {
		t.Errorf("HTTPStatus = %d; want 410", stub.HTTPStatus)
	}
	if stub.Details.FinalBranchName != nil {
		t.Errorf("Details.FinalBranchName = %v; want nil (omitempty)", stub.Details.FinalBranchName)
	}
	if stub.Details.EndReason != "timeout" {
		t.Errorf("Details.EndReason = %q; want \"timeout\"", stub.Details.EndReason)
	}
}

// containsStr is a simple helper to avoid importing strings in test file.
func containsStr(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || func() bool {
		for i := 0; i <= len(s)-len(sub); i++ {
			if s[i:i+len(sub)] == sub {
				return true
			}
		}
		return false
	}())
}
