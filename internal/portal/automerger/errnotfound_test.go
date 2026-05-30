package automerger_test

// errnotfound_test.go — regression tests for Unit 3: errors.Is for store.ErrNotFound.
//
// Bug: worker.go refModeForSession and outcomes.go tryResolveConflict used
// == / != comparisons against store.ErrNotFound. When any layer wraps the
// sentinel with %w (as the rest of the codebase routinely does), the comparison
// becomes false — treating a missing ref-mode as a hard error (worker.go) or
// a missing conflict event as a real failure (outcomes.go).
//
// Fix: use errors.Is(err, store.ErrNotFound) at both sites.
//
// Test strategy: use a stub store that returns fmt.Errorf("wrapped: %w",
// store.ErrNotFound) for GetRefMode / GetConflictEventByID and assert the
// correct not-found behaviour at each call site.

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
)

// ---------------------------------------------------------------------------
// wrappedNotFoundRefModeStore — wraps ErrNotFound in GetRefMode
// ---------------------------------------------------------------------------

// wrappedNotFoundRefModeStore wraps a real store.Store and returns a
// %w-wrapped ErrNotFound from GetRefMode to simulate what a middleware
// layer might produce.
type wrappedNotFoundRefModeStore struct {
	store.Store
}

func (w *wrappedNotFoundRefModeStore) GetRefMode(_ context.Context, _ store.GetRefModeParams) (store.RefMode, error) {
	return store.RefMode{}, fmt.Errorf("lookup layer: %w", store.ErrNotFound)
}

// ---------------------------------------------------------------------------
// TestWorker_RefMode_WrappedErrNotFound_FallsBackToDefault
// ---------------------------------------------------------------------------

// TestWorker_RefMode_WrappedErrNotFound_FallsBackToDefault verifies that when
// GetRefMode returns a wrapped ErrNotFound, the worker falls back to the
// session's DefaultMode rather than treating it as a hard error.
//
// If the old != comparison were still in place, the worker would return an
// error from refModeForSession and skip the merge entirely.
func TestWorker_RefMode_WrappedErrNotFound_FallsBackToDefault(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := openWorkerStore(t)
	wrappedStore := &wrappedNotFoundRefModeStore{Store: s}

	log := events.New(s)

	orgID := "org-errnotfound-1"
	sessID := "sess-errnotfound-1"
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: orgID, Slug: orgID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessID,
		OrgID:         orgID,
		Name:          sessID,
		Goal:          "errnotfound-test",
		WritableScope: `["**"]`,
		DefaultMode:   "sync", // worker should fall back to this
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.EnsureEventSeqRow(ctx, sessID); err != nil {
		t.Fatalf("EnsureEventSeqRow: %v", err)
	}

	// Build a clean-merge repo.
	repo, repoDir := initRepo(t)
	ancestor := commitFiles(t, repo, repoDir, nil, map[string][]byte{"f.txt": []byte("base\n")}, "base")
	draftTip := commitFiles(t, repo, repoDir, ancestor, map[string][]byte{
		"f.txt": []byte("base\n"), "a.txt": []byte("draft\n"),
	}, "draft")
	source := commitFiles(t, repo, repoDir, ancestor, map[string][]byte{
		"f.txt": []byte("base\n"), "b.txt": []byte("source\n"),
	}, "source")

	draftRef := plumbing.NewBranchReferenceName("jam/" + sessID + "/draft")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(draftRef, draftTip.Hash)); err != nil {
		t.Fatalf("set draft ref: %v", err)
	}

	sto := &singleRepoStorage{repoDir: repoDir}
	applier := automerger.NewApplier(s, log)

	mergeCh, unsub := log.Subscribe("merge.succeeded")
	defer unsub()

	w := &automerger.Worker{
		Store:       wrappedStore,
		Storage:     sto,
		Log:         log,
		Applier:     applier,
		PortalHost:  "test.jamsesh.local",
		IdleTimeout: 200 * time.Millisecond,
		QueueSize:   256,
	}
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	payload := buildCommitArrivedPayload(t, "refs/heads/jam/"+sessID+"/alice/feat", source.Hash.String())
	if _, err := log.Emit(ctx, orgID, sessID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// The worker should fall back to DefaultMode=sync and produce merge.succeeded.
	select {
	case e := <-mergeCh:
		if e.SessionID != sessID {
			t.Errorf("merge.succeeded session: want %s, got %s", sessID, e.SessionID)
		}
		// Good — wrapped ErrNotFound was treated as not-found, not hard error.
	case <-time.After(10 * time.Second):
		t.Error("timed out: wrapped ErrNotFound in GetRefMode was treated as a hard error (regression of == comparison)")
	}
}

// ---------------------------------------------------------------------------
// wrappedNotFoundConflictStore — wraps ErrNotFound in GetConflictEventByID
// ---------------------------------------------------------------------------

// wrappedNotFoundConflictStore wraps a real store.Store and returns a
// %w-wrapped ErrNotFound from GetConflictEventByID.
type wrappedNotFoundConflictStore struct {
	store.Store
}

func (w *wrappedNotFoundConflictStore) GetConflictEventByID(_ context.Context, _ string) (store.ConflictEvent, error) {
	return store.ConflictEvent{}, fmt.Errorf("lookup layer: %w", store.ErrNotFound)
}

// ---------------------------------------------------------------------------
// TestApply_TryResolveConflict_WrappedErrNotFound_IsNoOp
// ---------------------------------------------------------------------------

// TestApply_TryResolveConflict_WrappedErrNotFound_IsNoOp verifies that when
// GetConflictEventByID returns a wrapped ErrNotFound, tryResolveConflict treats
// it as a silent no-op (not an error), consistent with errors.Is semantics.
func TestApply_TryResolveConflict_WrappedErrNotFound_IsNoOp(t *testing.T) {
	ctx := context.Background()

	// Use the wrappedNotFoundConflictStore for the Applier's store.
	s := openTestStore(t)
	wrappedStore := &wrappedNotFoundConflictStore{Store: s}

	sess := seedSession(t, s)
	if err := s.EnsureEventSeqRow(ctx, sess.ID); err != nil {
		t.Fatalf("ensure event_seq row: %v", err)
	}

	log := events.New(s)

	// Build a clean-merge repo with a Resolves-Conflict trailer pointing at a
	// nonexistent event — GetConflictEventByID will return wrapped ErrNotFound.
	repo, repoDir := initRepo(t)
	base := commitFiles(t, repo, repoDir, nil, map[string][]byte{"file.txt": []byte("base\n")}, "base")
	draft := commitFiles(t, repo, repoDir, base, map[string][]byte{
		"file.txt": []byte("base\n"), "extra.txt": []byte("extra\n"),
	}, "draft")

	sourceMsg := "source: fix\n\nResolves-Conflict: nonexistent-event-wrapped\n"
	source := commitFilesWithMessage(t, repo, repoDir, base, map[string][]byte{
		"file.txt": []byte("base\n"), "file-s.txt": []byte("src\n"),
	}, sourceMsg)

	result := runMerge(t, repo, source.Hash, draft.Hash, base.Hash)
	if result.Kind != automerger.CleanMerge {
		t.Fatalf("expected CleanMerge, got %s", result.Kind)
	}

	applier := automerger.NewApplier(wrappedStore, log)
	_, err := applier.Apply(ctx, automerger.ApplyInput{
		Repo:         repo,
		Session:      sess,
		SourceRef:    "refs/heads/jam/sess-1/alice/feat",
		SourceCommit: source.Hash,
		DraftTip:     draft.Hash,
		Ancestor:     base.Hash,
		Result:       result,
		PortalHost:   "jamsesh.test",
	})

	// Must NOT return an error — wrapped ErrNotFound is a silent no-op.
	if err != nil {
		t.Errorf("Apply returned error for wrapped ErrNotFound in GetConflictEventByID: %v"+
			" (regression of == comparison that failed to unwrap)", err)
	}

	// Verify errors.Is behaviour works for wrapped ErrNotFound.
	wrappedErr := fmt.Errorf("lookup layer: %w", store.ErrNotFound)
	if !errors.Is(wrappedErr, store.ErrNotFound) {
		t.Error("errors.Is(wrapped, ErrNotFound) should be true — sentinel is not being unwrapped")
	}
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// singleRepoStorage returns the same repoDir for any (orgID, sessID).
type singleRepoStorage struct {
	repoDir string
}

func (s *singleRepoStorage) RepoPath(_, _ string) string { return s.repoDir }
func (s *singleRepoStorage) CreateRepo(_ context.Context, _, _ string) error { return nil }
func (s *singleRepoStorage) RemoveRepo(_ context.Context, _, _ string) error { return nil }
func (s *singleRepoStorage) RepoExists(_, _ string) (bool, error)            { return true, nil }
func (s *singleRepoStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (s *singleRepoStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, nil
}
func (s *singleRepoStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

var _ storage.Service = (*singleRepoStorage)(nil)

// Suppress unused imports.
var _ *gogit.Repository
var _ *object.Commit
var _ plumbing.Hash
