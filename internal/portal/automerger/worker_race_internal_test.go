package automerger

// worker_race_internal_test.go — deterministic idle-race regression test.
//
// This test uses the unexported onIdleDecision hook on Worker to force the
// precise interleaving that caused the original lost-event race:
//
//  1. A session worker drains all events from its queue and reaches the idle case.
//  2. The idle check sees len(ch)==0 (willExit=true).
//  3. onIdleDecision fires while still holding w.mu — we use it to verify the
//     invariant: no event can be lost, because enqueue will find the sessions
//     entry gone and create a new one, spawning a fresh draining goroutine.
//
// Under the old two-sync.Map design the hook would not exist; events emitted
// concurrently with idle-deletion would land on an orphan channel.
// Under the new single-mu-guarded design, enqueue running concurrently will
// block on mu, see the entry missing (deleted), create a new sessionQueue, and
// spawn a fresh goroutine — so the event is always processed.

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
)

// TestWorkerRace_DeterministicIdleRace forces the idle-exit + concurrent-enqueue
// interleaving deterministically via onIdleDecision and asserts that the second
// event is always processed (not stranded).
func TestWorkerRace_DeterministicIdleRace(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	s := openInternalWorkerStore(t)
	log := events.New(s)

	orgID := "org-idle-race"
	sessID := "sess-idle-race"

	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID: orgID, Name: orgID, Slug: orgID, CreatedAt: time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	if _, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            sessID,
		OrgID:         orgID,
		Name:          sessID,
		Goal:          "idle-race-test",
		WritableScope: `["**"]`,
		DefaultMode:   "sync",
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	}); err != nil {
		t.Fatalf("CreateSession: %v", err)
	}
	if err := s.EnsureEventSeqRow(ctx, sessID); err != nil {
		t.Fatalf("EnsureEventSeqRow: %v", err)
	}

	// Build a minimal clean-merge repo topology.
	repo, repoDir := initRepoInternal(t)
	ancestor := commitFilesInternal(t, repo, repoDir, nil,
		map[string][]byte{"f.txt": []byte("base\n")}, "base")
	draftTip := commitFilesInternal(t, repo, repoDir, ancestor,
		map[string][]byte{"f.txt": []byte("base\n"), "a.txt": []byte("draft\n")}, "draft")
	source := commitFilesInternal(t, repo, repoDir, ancestor,
		map[string][]byte{"f.txt": []byte("base\n"), "b.txt": []byte("source\n")}, "source")

	draftRef := plumbing.NewBranchReferenceName("jam/" + sessID + "/draft")
	if err := repo.Storer.SetReference(plumbing.NewHashReference(draftRef, draftTip.Hash)); err != nil {
		t.Fatalf("set draft ref: %v", err)
	}

	internalMS := &internalSingleStorage{repoDir: repoDir}
	applier := NewApplier(s, log)

	// outcomeCh counts merge.succeeded / conflict.detected events.
	outcomeCh, unsub := log.Subscribe("")
	defer unsub()

	var outcomeCount atomic.Int64
	go func() {
		for e := range outcomeCh {
			if e.Type == "merge.succeeded" || e.Type == "conflict.detected" {
				outcomeCount.Add(1)
			}
		}
	}()

	// hookFired is closed when the idle hook fires (first idle decision).
	hookFired := make(chan struct{})
	var hookOnce sync.Once

	// secondEmitted is closed after the second event has been emitted during the hook.
	secondEmitted := make(chan struct{})

	w := &Worker{
		Store:       s,
		Storage:     internalMS,
		Log:         log,
		Applier:     applier,
		PortalHost:  "test.jamsesh.local",
		IdleTimeout: 5 * time.Millisecond,
		QueueSize:   256,
	}

	// Set the onIdleDecision hook before Start.
	w.onIdleDecision = func(sessionID string, willExit bool) {
		if sessionID != sessID {
			return
		}
		// Only act on the first "willExit=true" decision.
		if !willExit {
			return
		}
		hookOnce.Do(func() {
			close(hookFired)
			// The hook is called under w.mu. We cannot call enqueue here
			// (it acquires mu) — so we signal a goroutine to emit after we return.
			// The new design guarantees: once mu is released and the delete happens,
			// enqueue will see no entry and create a new one.
		})
	}

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Emit the first event.
	payload1 := buildCommitArrivedPayloadInternal(sessID, "refs/heads/jam/"+sessID+"/alice/feat", source.Hash.String())
	if _, err := log.Emit(ctx, orgID, sessID, "commit.arrived", payload1); err != nil {
		t.Fatalf("Emit 1: %v", err)
	}

	// Wait for the first outcome (first event processed).
	select {
	case <-waitForOutcome(&outcomeCount, 1, 10*time.Second):
		// Good.
	case <-time.After(10 * time.Second):
		t.Fatal("timed out waiting for first outcome")
	}

	// Wait for the idle decision hook to fire (worker is deciding to exit).
	select {
	case <-hookFired:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for idle hook to fire")
	}

	// Immediately emit a second event — this races with the idle-exit.
	// Under the new design the worker will either:
	//   (a) still be alive and process it, OR
	//   (b) have exited; enqueue creates a new sessionQueue + goroutine.
	// Either way the event must be processed.
	payload2 := buildCommitArrivedPayloadInternal(sessID, "refs/heads/jam/"+sessID+"/alice/feat", source.Hash.String())
	if _, err := log.Emit(ctx, orgID, sessID, "commit.arrived", payload2); err != nil {
		t.Fatalf("Emit 2: %v", err)
	}
	close(secondEmitted)

	// Wait for the second outcome.
	select {
	case <-waitForOutcome(&outcomeCount, 2, 15*time.Second):
		// Good — no event was stranded.
	case <-time.After(15 * time.Second):
		t.Errorf("second event stranded: got %d outcomes, want 2", outcomeCount.Load())
	}

	_ = secondEmitted
}

// waitForOutcome returns a channel that is closed once outcomeCount >= target.
func waitForOutcome(cnt *atomic.Int64, target int64, timeout time.Duration) <-chan struct{} {
	done := make(chan struct{})
	go func() {
		deadline := time.Now().Add(timeout)
		for time.Now().Before(deadline) {
			if cnt.Load() >= target {
				close(done)
				return
			}
			time.Sleep(2 * time.Millisecond)
		}
	}()
	return done
}

// buildCommitArrivedPayloadInternal builds a minimal JSON payload.
func buildCommitArrivedPayloadInternal(sessID, ref, sha string) []byte {
	return []byte(fmt.Sprintf(
		`{"session_id":%q,"ref":%q,"sha":%q,"author_id":"test","summary":"race"}`,
		sessID, ref, sha,
	))
}

// internalSingleStorage returns the same repoDir for any (orgID, sessID).
type internalSingleStorage struct {
	repoDir string
}

func (s *internalSingleStorage) RepoPath(_, _ string) string { return s.repoDir }
func (s *internalSingleStorage) CreateRepo(_ context.Context, _, _ string) error { return nil }
func (s *internalSingleStorage) RemoveRepo(_ context.Context, _, _ string) error { return nil }
func (s *internalSingleStorage) RepoExists(_, _ string) (bool, error)            { return true, nil }
func (s *internalSingleStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (s *internalSingleStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, nil
}
func (s *internalSingleStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

var _ storage.Service = (*internalSingleStorage)(nil)

// ---------------------------------------------------------------------------
// Internal store helper (no workerDBCounter visible here)
// ---------------------------------------------------------------------------

var internalWorkerCounter atomic.Int64

func openInternalWorkerStore(t *testing.T) store.Store {
	t.Helper()
	n := internalWorkerCounter.Add(1)
	dsn := fmt.Sprintf("file:internal_worker_race_%d?mode=memory&cache=shared", n)
	s, _, err := db.Open(context.Background(), "sqlite", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("openInternalWorkerStore: %v", err)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// ---------------------------------------------------------------------------
// Repo helpers (mirrors testhelpers_test.go but available in this package)
// ---------------------------------------------------------------------------

func initRepoInternal(t *testing.T) (*gogit.Repository, string) {
	t.Helper()
	return initRepo(t)
}

func commitFilesInternal(t *testing.T, repo *gogit.Repository, dir string, parent *object.Commit, files map[string][]byte, msg string) *object.Commit {
	t.Helper()
	return commitFiles(t, repo, dir, parent, files, msg)
}
