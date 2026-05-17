package finalize_test

import (
	"context"
	"encoding/json"
	"fmt"
	"path/filepath"
	"strings"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/oklog/ulid/v2"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// planEnv is plan-test-specific scaffolding: a real bare repo on disk,
// commits inserted via go-git's object store, and the in-memory store +
// finalize handler wired against them.
type planEnv struct {
	env *finalizeEnv

	repoDir string
	repo    *gogit.Repository

	// helper to write commits and return their hashes.
	commitMessages map[string]string
}

// fsStorage is a real-repo-on-disk storage stub. It satisfies the subset
// of storage.Service the finalize handler uses; the bare repo is created
// in t.TempDir so each test gets a fresh disk path.
type fsStorage struct {
	root string
}

func (s *fsStorage) RepoPath(orgID, sessionID string) string {
	return filepath.Join(s.root, orgID, sessionID)
}
func (s *fsStorage) CreateRepo(_ context.Context, _, _ string) error  { return nil }
func (s *fsStorage) RemoveRepo(_ context.Context, _, _ string) error  { return nil }
func (s *fsStorage) RepoExists(_, _ string) (bool, error)             { return false, nil }
func (s *fsStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (s *fsStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, store.ErrNotFound
}
func (s *fsStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

// newPlanEnv constructs an env with: in-memory sqlite store, an empty
// real-on-disk bare repo at <tmp>/<orgID>/<sessionID>, and the finalize
// handler wired against both. Caller and "other" accounts exist and are
// session members; the session is in "active" status with goal "test goal".
func newPlanEnv(t *testing.T) *planEnv {
	t.Helper()

	ctx := context.Background()
	s, err := db.Open(ctx, "sqlite", ":memory:", db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })

	log := events.New(s)
	tokSvc := tokens.New(s)

	root := t.TempDir()
	storageSvc := &fsStorage{root: root}
	handler := finalize.New(s, storageSvc, log, tokSvc, "https://portal.test")

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
			t.Fatalf("add org member: %v", err)
		}
	}

	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID: sessID, OrgID: orgID, Name: "finalize-plan-test",
		Goal: "test goal", WritableScope: `["**"]`, DefaultMode: "sync",
		Status: "active", CreatedAt: now,
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
			t.Fatalf("add session member: %v", err)
		}
	}

	other, _ := s.GetAccountByID(ctx, otherID)

	finalEnv := &finalizeEnv{
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

	// Create a real bare repo at storageSvc.RepoPath(orgID, sessID).
	repoPath := storageSvc.RepoPath(orgID, sessID)
	repo, err := gogit.PlainInit(repoPath, true)
	if err != nil {
		t.Fatalf("PlainInit bare: %v", err)
	}

	return &planEnv{
		env:            finalEnv,
		repoDir:        repoPath,
		repo:           repo,
		commitMessages: map[string]string{},
	}
}

// putCommit writes a synthetic commit (empty tree, fixed timestamp +
// offset) into the bare repo and returns its hash string. The author and
// committer come from authorName/authorEmail.
func (pe *planEnv) putCommit(t *testing.T, authorName, authorEmail, message string, parents []plumbing.Hash, idxMinute int) plumbing.Hash {
	t.Helper()
	when := time.Date(2026, 5, 17, 12, idxMinute, 0, 0, time.UTC)
	sig := object.Signature{Name: authorName, Email: authorEmail, When: when}

	// Empty tree.
	tree := &object.Tree{}
	te := pe.repo.Storer.NewEncodedObject()
	te.SetType(plumbing.TreeObject)
	if err := tree.Encode(te); err != nil {
		t.Fatalf("encode tree: %v", err)
	}
	treeHash, err := pe.repo.Storer.SetEncodedObject(te)
	if err != nil {
		t.Fatalf("store tree: %v", err)
	}

	commit := &object.Commit{
		Author:       sig,
		Committer:    sig,
		Message:      message,
		TreeHash:     treeHash,
		ParentHashes: parents,
	}
	ce := pe.repo.Storer.NewEncodedObject()
	ce.SetType(plumbing.CommitObject)
	if err := commit.Encode(ce); err != nil {
		t.Fatalf("encode commit: %v", err)
	}
	hash, err := pe.repo.Storer.SetEncodedObject(ce)
	if err != nil {
		t.Fatalf("store commit: %v", err)
	}
	pe.commitMessages[hash.String()] = message
	return hash
}

// seedLockWithCuration inserts a lock row with the given curation state and
// returns its id. Lock is held by env.caller.
func (pe *planEnv) seedLockWithCuration(t *testing.T, shas []string, mode, target, base string, msg *string) string {
	t.Helper()
	ctx := context.Background()
	id := ulid.Make().String()
	now := time.Now().UTC()
	shasJSON, _ := json.Marshal(shas)
	if err := pe.env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  id,
		OrgID:               pe.env.orgID,
		SessionID:           pe.env.sessID,
		AcquiredByAccountID: pe.env.caller.ID,
		AcquiredAt:          now,
		LastActivityAt:      now,
		SelectedCommitSHAs:  string(shasJSON),
		TargetBranch:        target,
		BaseSHA:             base,
		Mode:                mode,
		CommitMessage:       msg,
	}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}
	return id
}

// ---------- Tests ----------

func TestGetFinalizePlan_HappySquash(t *testing.T) {
	pe := newPlanEnv(t)

	hA := pe.putCommit(t, "Alice", "alice@example.com", "feat: A\n", nil, 1)
	hB := pe.putCommit(t, "Bob", "bob@example.com", "feat: B\n", []plumbing.Hash{hA}, 2)

	lockID := pe.seedLockWithCuration(t, []string{hA.String(), hB.String()},
		"squash", "ship/foo", hA.String(), nil)

	resp, err := pe.env.handler.GetFinalizePlan(pe.env.callerCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: lockID},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T: %+v", resp, resp)
	}

	wantPlanID := pe.env.sessID + ":" + lockID
	if got.PlanId != wantPlanID {
		t.Errorf("PlanId = %q, want %q", got.PlanId, wantPlanID)
	}
	if got.Mode != openapi.PlanMode("squash") {
		t.Errorf("Mode = %v, want squash", got.Mode)
	}
	if len(got.SelectedCommits) != 2 {
		t.Fatalf("SelectedCommits len = %d, want 2", len(got.SelectedCommits))
	}
	if got.SelectedCommits[0].Sha != hA.String() {
		t.Errorf("SelectedCommits[0].Sha = %s, want %s", got.SelectedCommits[0].Sha, hA)
	}
	if got.SelectedCommits[0].AuthorName != "Alice" {
		t.Errorf("SelectedCommits[0].AuthorName = %q, want Alice", got.SelectedCommits[0].AuthorName)
	}
	if got.CommitMessage == "" {
		t.Error("squash mode: CommitMessage should be populated")
	}
	if len(got.CoAuthors) != 2 {
		t.Errorf("CoAuthors len = %d, want 2", len(got.CoAuthors))
	}

	if got.Script == "" {
		t.Error("Script should not be empty")
	}
	if !strings.Contains(got.Script, "set -euo pipefail") {
		t.Errorf("Script missing set -euo pipefail; got:\n%s", got.Script)
	}
	if !strings.Contains(got.Script, "$JAMSESH_FETCH_REMOTE") {
		t.Errorf("Script missing $JAMSESH_FETCH_REMOTE")
	}
	if !strings.Contains(got.Script, hA.String()) || !strings.Contains(got.Script, hB.String()) {
		t.Errorf("Script missing curated SHAs")
	}

	if got.FetchSource.Kind != openapi.Https {
		t.Errorf("FetchSource.Kind = %v, want https", got.FetchSource.Kind)
	}
	wantURL := fmt.Sprintf("https://portal.test/git/%s/%s.git", pe.env.orgID, pe.env.sessID)
	if got.FetchSource.RemoteUrl != wantURL {
		t.Errorf("FetchSource.RemoteUrl = %q, want %q", got.FetchSource.RemoteUrl, wantURL)
	}

	if got.LockStatus.LockId != lockID {
		t.Errorf("LockStatus.LockId = %q, want %q", got.LockStatus.LockId, lockID)
	}
	if !got.LockStatus.IsCaller {
		t.Error("LockStatus.IsCaller = false, want true")
	}
	if got.TargetBranch != "ship/foo" {
		t.Errorf("TargetBranch = %q, want ship/foo", got.TargetBranch)
	}
	if got.BaseSha != hA.String() {
		t.Errorf("BaseSha = %q, want %q", got.BaseSha, hA.String())
	}
}

func TestGetFinalizePlan_HappyPreserve(t *testing.T) {
	pe := newPlanEnv(t)

	hA := pe.putCommit(t, "Alice", "alice@example.com", "feat: A\n", nil, 1)
	hB := pe.putCommit(t, "Bob", "bob@example.com", "feat: B\n", []plumbing.Hash{hA}, 2)

	lockID := pe.seedLockWithCuration(t, []string{hA.String(), hB.String()},
		"preserve", "ship/foo", hA.String(), nil)

	resp, err := pe.env.handler.GetFinalizePlan(pe.env.callerCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: lockID},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan200JSONResponse)
	if !ok {
		t.Fatalf("expected 200, got %T: %+v", resp, resp)
	}

	if got.Mode != openapi.PlanMode("preserve") {
		t.Errorf("Mode = %v, want preserve", got.Mode)
	}
	if got.CommitMessage != "" {
		t.Errorf("preserve mode: CommitMessage should be empty, got %q", got.CommitMessage)
	}
	if len(got.CoAuthors) != 0 {
		t.Errorf("preserve mode: CoAuthors should be empty, got %d entries", len(got.CoAuthors))
	}
	// Preserve script uses individual cherry-picks (no --no-commit).
	if strings.Contains(got.Script, "--no-commit") {
		t.Error("preserve script should not contain --no-commit")
	}
	if !strings.Contains(got.Script, "git cherry-pick "+hA.String()) {
		t.Errorf("preserve script missing 'git cherry-pick %s'", hA)
	}
}

func TestGetFinalizePlan_MissingCommit_409(t *testing.T) {
	pe := newPlanEnv(t)

	hA := pe.putCommit(t, "Alice", "alice@example.com", "feat: A\n", nil, 1)
	missing := "deadbeefdeadbeefdeadbeefdeadbeefdeadbeef"
	lockID := pe.seedLockWithCuration(t, []string{hA.String(), missing},
		"squash", "ship/foo", hA.String(), nil)

	resp, err := pe.env.handler.GetFinalizePlan(pe.env.callerCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: lockID},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T: %+v", resp, resp)
	}
	if got.Error != "finalize.commit_missing" {
		t.Errorf("Error = %q, want finalize.commit_missing", got.Error)
	}
	if got.Details == nil || got.Details["missing_sha"] != missing {
		t.Errorf("Details.missing_sha = %v, want %q", got.Details, missing)
	}
}

func TestGetFinalizePlan_LockExpired_409(t *testing.T) {
	pe := newPlanEnv(t)

	// Seed an idle 31-min-old lock.
	ctx := context.Background()
	id := ulid.Make().String()
	old := time.Now().UTC().Add(-31 * time.Minute)
	if err := pe.env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID:                  id,
		OrgID:               pe.env.orgID,
		SessionID:           pe.env.sessID,
		AcquiredByAccountID: pe.env.caller.ID,
		AcquiredAt:          old,
		LastActivityAt:      old,
		SelectedCommitSHAs:  "[]",
		Mode:                "squash",
	}); err != nil {
		t.Fatalf("seed idle lock: %v", err)
	}

	resp, err := pe.env.handler.GetFinalizePlan(pe.env.callerCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: id},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T: %+v", resp, resp)
	}
	if got.Error != "finalize.lock_expired" {
		t.Errorf("Error = %q, want finalize.lock_expired", got.Error)
	}
}

func TestGetFinalizePlan_LockSuperseded_409(t *testing.T) {
	pe := newPlanEnv(t)

	// Seed two locks; mark the first as superseded by the second.
	ctx := context.Background()
	first := ulid.Make().String()
	second := ulid.Make().String()
	now := time.Now().UTC()
	if err := pe.env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID: first, OrgID: pe.env.orgID, SessionID: pe.env.sessID,
		AcquiredByAccountID: pe.env.caller.ID,
		AcquiredAt:          now, LastActivityAt: now,
		SelectedCommitSHAs: "[]", Mode: "squash",
	}); err != nil {
		t.Fatalf("seed first: %v", err)
	}
	if err := pe.env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID: second, OrgID: pe.env.orgID, SessionID: pe.env.sessID,
		AcquiredByAccountID: pe.env.otherID,
		AcquiredAt:          now, LastActivityAt: now,
		SelectedCommitSHAs: "[]", Mode: "squash",
	}); err != nil {
		t.Fatalf("seed second: %v", err)
	}
	if err := pe.env.store.SupersedeFinalizeLock(ctx, store.SupersedeFinalizeLockParams{
		ID: first, SupersededByLockID: second,
	}); err != nil {
		t.Fatalf("supersede: %v", err)
	}

	resp, err := pe.env.handler.GetFinalizePlan(pe.env.callerCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: first},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan409JSONResponse)
	if !ok {
		t.Fatalf("expected 409, got %T: %+v", resp, resp)
	}
	if got.Error != "finalize.lock_superseded" {
		t.Errorf("Error = %q, want finalize.lock_superseded", got.Error)
	}
	if got.Details == nil || got.Details["superseded_by_lock_id"] != second {
		t.Errorf("Details.superseded_by_lock_id = %v, want %q", got.Details, second)
	}
}

func TestGetFinalizePlan_LockBelongsToDifferentSession_404(t *testing.T) {
	pe := newPlanEnv(t)

	// Create a SECOND session in the same org; the lock will be bound to it.
	ctx := context.Background()
	otherSessID := ulid.Make().String()
	now := time.Now().UTC()
	if _, err := pe.env.store.CreateSession(ctx, store.CreateSessionParams{
		ID: otherSessID, OrgID: pe.env.orgID, Name: "other-session",
		Goal: "other", WritableScope: `["**"]`, DefaultMode: "sync",
		Status: "active", CreatedAt: now,
	}); err != nil {
		t.Fatalf("create other session: %v", err)
	}
	if err := pe.env.store.AddSessionMember(ctx, store.AddSessionMemberParams{
		OrgID: pe.env.orgID, SessionID: otherSessID,
		AccountID: pe.env.caller.ID, Role: "creator", JoinedAt: now,
	}); err != nil {
		t.Fatalf("add other session member: %v", err)
	}

	lockID := ulid.Make().String()
	if err := pe.env.store.InsertFinalizeLock(ctx, store.InsertFinalizeLockParams{
		ID: lockID, OrgID: pe.env.orgID, SessionID: otherSessID,
		AcquiredByAccountID: pe.env.caller.ID,
		AcquiredAt:          now, LastActivityAt: now,
		SelectedCommitSHAs: "[]", Mode: "squash",
	}); err != nil {
		t.Fatalf("seed lock: %v", err)
	}

	// Look it up via the FIRST session's id — should be 404 (we don't leak).
	resp, err := pe.env.handler.GetFinalizePlan(pe.env.callerCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID, // mismatched
		Params:    openapi.GetFinalizePlanParams{LockId: lockID},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan404JSONResponse)
	if !ok {
		t.Fatalf("expected 404, got %T: %+v", resp, resp)
	}
	if got.Error != "finalize.lock_not_found" {
		t.Errorf("Error = %q, want finalize.lock_not_found", got.Error)
	}
}

func TestGetFinalizePlan_NonMember_403(t *testing.T) {
	pe := newPlanEnv(t)

	// Create a third account that is NOT a session member.
	ctx := context.Background()
	outsiderID := ulid.Make().String()
	now := time.Now().UTC()
	outsider, err := pe.env.store.CreateAccount(ctx, store.CreateAccountParams{
		ID: outsiderID, Email: "outsider@example.com",
		DisplayName: "Outsider", CreatedAt: now,
	})
	if err != nil {
		t.Fatalf("create outsider: %v", err)
	}
	if err := pe.env.store.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID: pe.env.orgID, AccountID: outsiderID, Role: "member", CreatedAt: now,
	}); err != nil {
		t.Fatalf("add org member outsider: %v", err)
	}
	// Intentionally NOT a session member.

	outsiderCtx := tokens.ContextWithAccount(ctx, &outsider)

	lockID := pe.seedLockWithCuration(t, []string{}, "squash", "x", "y", nil)

	resp, err := pe.env.handler.GetFinalizePlan(outsiderCtx, openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: lockID},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	got, ok := resp.(openapi.GetFinalizePlan403JSONResponse)
	if !ok {
		t.Fatalf("expected 403, got %T: %+v", resp, resp)
	}
	if got.Error != "auth.insufficient_permission" {
		t.Errorf("Error = %q, want auth.insufficient_permission", got.Error)
	}
}

func TestGetFinalizePlan_Unauthenticated_401(t *testing.T) {
	pe := newPlanEnv(t)
	lockID := pe.seedLockWithCuration(t, []string{}, "squash", "x", "y", nil)

	// Plain background context — no account.
	resp, err := pe.env.handler.GetFinalizePlan(context.Background(), openapi.GetFinalizePlanRequestObject{
		OrgID:     pe.env.orgID,
		SessionID: pe.env.sessID,
		Params:    openapi.GetFinalizePlanParams{LockId: lockID},
	})
	if err != nil {
		t.Fatalf("GetFinalizePlan: %v", err)
	}
	if _, ok := resp.(openapi.GetFinalizePlan401JSONResponse); !ok {
		t.Fatalf("expected 401, got %T: %+v", resp, resp)
	}
}

