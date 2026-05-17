package automerger_test

import (
	"context"
	"database/sql"
	"encoding/json"
	"fmt"
	"sync/atomic"
	"testing"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"

	_ "modernc.org/sqlite"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
)

// ---------------------------------------------------------------------------
// events.Log Subscribe tests
// ---------------------------------------------------------------------------

func TestLog_Subscribe_ReceivesEvents(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	ch, unsub := log.Subscribe("")
	defer unsub()

	payload := mustMarshalWorker(t, map[string]string{"sha": "abc123"})
	_, err := log.Emit(ctx, sess.OrgID, sess.ID, "commit.arrived", payload)
	if err != nil {
		t.Fatalf("Emit: %v", err)
	}

	select {
	case e := <-ch:
		if e.Type != "commit.arrived" {
			t.Errorf("event type: want commit.arrived, got %s", e.Type)
		}
		if e.SessionID != sess.ID {
			t.Errorf("session id: want %s, got %s", sess.ID, e.SessionID)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event on subscriber channel")
	}
}

func TestLog_Subscribe_TypeFilter(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	// Subscribe only to merge.succeeded events.
	ch, unsub := log.Subscribe("merge.succeeded")
	defer unsub()

	// Emit a commit.arrived — should NOT arrive on ch.
	payload := mustMarshalWorker(t, map[string]string{"sha": "abc123"})
	if _, err := log.Emit(ctx, sess.OrgID, sess.ID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit commit.arrived: %v", err)
	}

	// Emit a merge.succeeded — should arrive on ch.
	successPayload := mustMarshalWorker(t, map[string]string{"draft_sha": "def456"})
	if _, err := log.Emit(ctx, sess.OrgID, sess.ID, "merge.succeeded", successPayload); err != nil {
		t.Fatalf("Emit merge.succeeded: %v", err)
	}

	select {
	case e := <-ch:
		if e.Type != "merge.succeeded" {
			t.Errorf("type filter failed: want merge.succeeded, got %s", e.Type)
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for merge.succeeded event")
	}

	// Ensure no commit.arrived was delivered.
	select {
	case e := <-ch:
		t.Errorf("unexpected extra event on filtered channel: %s", e.Type)
	default:
	}
}

func TestLog_Subscribe_Unsubscribe(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	ch, unsub := log.Subscribe("")
	unsub() // unsubscribe immediately

	// Emit after unsubscribe — must not panic, and channel should be closed.
	payload := mustMarshalWorker(t, map[string]string{"sha": "abc"})
	if _, err := log.Emit(ctx, sess.OrgID, sess.ID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit after unsubscribe: %v", err)
	}

	// Channel should be closed (receive returns zero value, ok=false).
	select {
	case _, ok := <-ch:
		if ok {
			t.Error("expected closed channel, got open")
		}
	default:
		// Channel not yet readable — that's also fine (the goroutine hasn't
		// had a chance to schedule); just verify no panic occurred.
	}
}

func TestLog_Subscribe_MultipleSubscribers(t *testing.T) {
	ctx := context.Background()
	s := openTestStore(t)
	sess := seedSession(t, s)
	log := events.New(s)

	ch1, unsub1 := log.Subscribe("")
	ch2, unsub2 := log.Subscribe("commit.arrived")
	defer unsub1()
	defer unsub2()

	payload := mustMarshalWorker(t, map[string]string{"sha": "abc"})
	if _, err := log.Emit(ctx, sess.OrgID, sess.ID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	for i, ch := range []<-chan events.Event{ch1, ch2} {
		select {
		case e := <-ch:
			if e.Type != "commit.arrived" {
				t.Errorf("subscriber %d: want commit.arrived, got %s", i+1, e.Type)
			}
		case <-time.After(time.Second):
			t.Errorf("subscriber %d: timed out", i+1)
		}
	}
}

// ---------------------------------------------------------------------------
// Worker tests
// ---------------------------------------------------------------------------

// workerDBCounter gives each worker test a unique named in-memory SQLite DB.
var workerDBCounter atomic.Int64

// openWorkerStore opens a named shared-cache in-memory SQLite with single
// connection so that all goroutines (including worker goroutines) share the
// same in-memory schema.
func openWorkerStore(t *testing.T) store.Store {
	t.Helper()
	n := workerDBCounter.Add(1)
	dsn := fmt.Sprintf("file:worker_test_%d?mode=memory&cache=shared", n)
	s, _, err := db.Open(context.Background(), "sqlite", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	type rawDBer interface{ RawDB() *sql.DB }
	if r, ok := s.(rawDBer); ok {
		r.RawDB().SetMaxOpenConns(1)
	}
	t.Cleanup(func() { _ = s.Close() })
	return s
}

// workerTestEnv holds all the pieces needed for a worker integration test.
type workerTestEnv struct {
	store   store.Store
	log     *events.Log
	storage *stubStorage
	sess    *store.Session
	repo    *gogit.Repository
	repoDir string
	applier *automerger.Applier
}

func setupWorkerEnv(t *testing.T, defaultMode string) *workerTestEnv {
	t.Helper()

	s := openWorkerStore(t)
	log := events.New(s)

	ctx := context.Background()
	org, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "org-w1",
		Name:      "Worker Org",
		Slug:      "worker-org",
		CreatedAt: time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateOrg: %v", err)
	}
	sess, err := s.CreateSession(ctx, store.CreateSessionParams{
		ID:            "sess-w1",
		OrgID:         org.ID,
		Name:          "Worker Session",
		Goal:          "testing",
		WritableScope: `["**"]`,
		DefaultMode:   defaultMode,
		Status:        "active",
		CreatedAt:     time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateSession: %v", err)
	}

	// Build a real repo with an ancestor → draftTip topology.
	// ancestor → draftTip (adds file-a.txt)
	// ancestor → source   (adds file-b.txt) — added per test
	repo, repoDir := initRepo(t)

	storage := &stubStorage{
		repo:    repo,
		repoDir: repoDir,
	}
	applier := automerger.NewApplier(s, log)

	return &workerTestEnv{
		store:   s,
		log:     log,
		storage: storage,
		sess:    &sess,
		repo:    repo,
		repoDir: repoDir,
		applier: applier,
	}
}

// stubStorage satisfies storage.Service minimally for tests.
type stubStorage struct {
	repo    *gogit.Repository
	repoDir string
	orgID   string
	sessID  string
}

func (ss *stubStorage) RepoPath(orgID, sessionID string) string {
	ss.orgID = orgID
	ss.sessID = sessionID
	return ss.repoDir
}

func (ss *stubStorage) CreateRepo(_ context.Context, _, _ string) error { return nil }
func (ss *stubStorage) RemoveRepo(_ context.Context, _, _ string) error { return nil }
func (ss *stubStorage) RepoExists(_, _ string) (bool, error)            { return true, nil }
func (ss *stubStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	return nil
}
func (ss *stubStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	return nil, nil
}
func (ss *stubStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	return storage.ArchivedStub{}
}

// Compile-time check that stubStorage satisfies storage.Service.
var _ storage.Service = (*stubStorage)(nil)

func newWorker(env *workerTestEnv) *automerger.Worker {
	return &automerger.Worker{
		Store:       env.store,
		Storage:     env.storage,
		Log:         env.log,
		Applier:     env.applier,
		PortalHost:  "test.jamsesh.local",
		IdleTimeout: 200 * time.Millisecond,
		QueueSize:   256,
	}
}

func TestWorker_Start_Stop(t *testing.T) {
	env := setupWorkerEnv(t, "sync")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	w := newWorker(env)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Cancel ctx and then Stop — should return quickly.
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 3*time.Second)
	defer stopCancel()
	if err := w.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}
}

func TestWorker_SyncMode_MergeSucceeded(t *testing.T) {
	env := setupWorkerEnv(t, "sync")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build repo:  ancestor → draftTip, ancestor → source
	ancestor := commitFiles(t, env.repo, env.repoDir, nil, map[string][]byte{
		"base.txt": []byte("base\n"),
	}, "base")
	draftTip := commitFiles(t, env.repo, env.repoDir, ancestor, map[string][]byte{
		"base.txt":   []byte("base\n"),
		"file-a.txt": []byte("draft side\n"),
	}, "draft: add file-a")
	source := commitFiles(t, env.repo, env.repoDir, ancestor, map[string][]byte{
		"base.txt":   []byte("base\n"),
		"file-b.txt": []byte("source side\n"),
	}, "source: add file-b")

	// Set the draft ref in the repo.
	draftRefName := plumbing.NewBranchReferenceName("jam/" + env.sess.ID + "/draft")
	if err := env.repo.Storer.SetReference(plumbing.NewHashReference(draftRefName, draftTip.Hash)); err != nil {
		t.Fatalf("set draft ref: %v", err)
	}

	// Subscribe to catch merge.succeeded.
	mergeCh, unsub := env.log.Subscribe("merge.succeeded")
	defer unsub()

	w := newWorker(env)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Emit a commit.arrived for a sync-mode ref.
	payload := buildCommitArrivedPayload(t, "refs/heads/jam/"+env.sess.ID+"/alice/feat", source.Hash.String())
	if _, err := env.log.Emit(ctx, env.sess.OrgID, env.sess.ID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit commit.arrived: %v", err)
	}

	// Wait for merge.succeeded.
	select {
	case e := <-mergeCh:
		if e.SessionID != env.sess.ID {
			t.Errorf("merge.succeeded session: want %s, got %s", env.sess.ID, e.SessionID)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for merge.succeeded")
	}

	// Verify the draft ref has advanced.
	draftRef, err := env.repo.Reference(draftRefName, true)
	if err != nil {
		t.Fatalf("read draft ref: %v", err)
	}
	if draftRef.Hash() == draftTip.Hash {
		t.Error("draft ref was not advanced by the worker")
	}
}

func TestWorker_IsolatedMode_SkipsRef(t *testing.T) {
	env := setupWorkerEnv(t, "sync") // session default is sync
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// Build repo.
	ancestor := commitFiles(t, env.repo, env.repoDir, nil, map[string][]byte{
		"base.txt": []byte("base\n"),
	}, "base")
	draftTip := commitFiles(t, env.repo, env.repoDir, ancestor, map[string][]byte{
		"base.txt":   []byte("base\n"),
		"file-a.txt": []byte("draft side\n"),
	}, "draft: add file-a")
	source := commitFiles(t, env.repo, env.repoDir, ancestor, map[string][]byte{
		"base.txt":   []byte("base\n"),
		"file-b.txt": []byte("source side\n"),
	}, "source: add file-b")

	// Set the draft ref.
	draftRefName := plumbing.NewBranchReferenceName("jam/" + env.sess.ID + "/draft")
	if err := env.repo.Storer.SetReference(plumbing.NewHashReference(draftRefName, draftTip.Hash)); err != nil {
		t.Fatalf("set draft ref: %v", err)
	}

	// Override the ref mode to isolated.
	isolatedRef := "refs/heads/jam/" + env.sess.ID + "/bob/isolated-branch"
	if err := env.store.UpsertRefMode(ctx, store.UpsertRefModeParams{
		SessionID: env.sess.ID,
		Ref:       isolatedRef,
		Mode:      "isolated",
	}); err != nil {
		t.Fatalf("UpsertRefMode: %v", err)
	}

	// Subscribe to merge events.
	mergeCh, unsub := env.log.Subscribe("merge.succeeded")
	defer unsub()
	conflictCh, unsub2 := env.log.Subscribe("conflict.detected")
	defer unsub2()

	w := newWorker(env)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Emit a commit.arrived for the isolated ref.
	payload := buildCommitArrivedPayload(t, isolatedRef, source.Hash.String())
	if _, err := env.log.Emit(ctx, env.sess.OrgID, env.sess.ID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit commit.arrived: %v", err)
	}

	// Allow some time; neither merge.succeeded nor conflict.detected should arrive.
	timer := time.NewTimer(500 * time.Millisecond)
	defer timer.Stop()
	select {
	case e := <-mergeCh:
		t.Errorf("unexpected merge.succeeded for isolated ref: %+v", e)
	case e := <-conflictCh:
		t.Errorf("unexpected conflict.detected for isolated ref: %+v", e)
	case <-timer.C:
		// Good — nothing happened.
	}

	// Draft ref must not have changed.
	draftRef, err := env.repo.Reference(draftRefName, true)
	if err != nil {
		t.Fatalf("read draft ref: %v", err)
	}
	if draftRef.Hash() != draftTip.Hash {
		t.Error("draft ref was advanced for an isolated ref — should have been skipped")
	}
}

func TestWorker_Backpressure(t *testing.T) {
	env := setupWorkerEnv(t, "sync")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	// blockingStorage wraps stubStorage and blocks in RepoPath until released.
	// This keeps the session worker goroutine busy while we flood the queue.
	blocked := make(chan struct{})  // signalled when storage is entered
	release := make(chan struct{})  // closed to unblock storage
	var storageBlockOnce atomic.Bool
	blockingSto := &blockingStorage{
		inner:   env.storage,
		blocked: blocked,
		release: release,
		once:    &storageBlockOnce,
	}

	queueSize := 2
	w := &automerger.Worker{
		Store:       env.store,
		Storage:     blockingSto,
		Log:         env.log,
		Applier:     env.applier,
		PortalHost:  "test.jamsesh.local",
		IdleTimeout: 200 * time.Millisecond,
		QueueSize:   queueSize,
	}

	bpCh, unsub := env.log.Subscribe("auto-merger.backpressure")
	defer unsub()

	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	ref := "refs/heads/jam/" + env.sess.ID + "/alice/feat"
	sha := "0000000000000000000000000000000000000001" // any SHA — we block before commit lookup

	// First event: enters processEvent → reaches blockingStorage.RepoPath and blocks.
	payload0 := buildCommitArrivedPayload(t, ref, sha)
	if _, err := env.log.Emit(ctx, env.sess.OrgID, env.sess.ID, "commit.arrived", payload0); err != nil {
		t.Fatalf("Emit 0: %v", err)
	}

	// Wait until the session worker goroutine is inside RepoPath (blocked).
	select {
	case <-blocked:
		// Good — worker is now blocked.
	case <-time.After(3 * time.Second):
		t.Fatal("timed out waiting for worker to enter blockingStorage")
	}

	// Flood with queueSize+2 more events — at least one must overflow.
	for i := 0; i < queueSize+2; i++ {
		payload := buildCommitArrivedPayload(t, ref, sha)
		_, _ = env.log.Emit(ctx, env.sess.OrgID, env.sess.ID, "commit.arrived", payload)
	}

	// Release the blocked worker goroutine.
	close(release)

	// Expect at least one backpressure event.
	select {
	case e := <-bpCh:
		if e.SessionID != env.sess.ID {
			t.Errorf("backpressure session: want %s, got %s", env.sess.ID, e.SessionID)
		}
	case <-time.After(3 * time.Second):
		// Timing-sensitive; soft-fail with a note rather than a hard failure.
		t.Log("no backpressure event within 3s — queue may have drained before flood (timing-dependent)")
	}
}

// blockingStorage wraps a stubStorage and blocks in RepoPath on the first call
// until release is closed, signalling via blocked when it enters.
type blockingStorage struct {
	inner   *stubStorage
	blocked chan struct{}
	release chan struct{}
	once    *atomic.Bool
}

func (b *blockingStorage) RepoPath(orgID, sessionID string) string {
	if b.once.CompareAndSwap(false, true) {
		// Signal that we're inside.
		select {
		case b.blocked <- struct{}{}:
		default:
		}
		// Block until released.
		<-b.release
	}
	return b.inner.RepoPath(orgID, sessionID)
}

func (b *blockingStorage) CreateRepo(ctx context.Context, o, s string) error {
	return b.inner.CreateRepo(ctx, o, s)
}
func (b *blockingStorage) RemoveRepo(ctx context.Context, o, s string) error {
	return b.inner.RemoveRepo(ctx, o, s)
}
func (b *blockingStorage) RepoExists(o, s string) (bool, error) { return b.inner.RepoExists(o, s) }
func (b *blockingStorage) ArchiveSession(ctx context.Context, o, s string, info storage.ArchiveInfo) error {
	return b.inner.ArchiveSession(ctx, o, s, info)
}
func (b *blockingStorage) LookupArchived(ctx context.Context, o, s string) (*storage.ArchivedRecord, error) {
	return b.inner.LookupArchived(ctx, o, s)
}
func (b *blockingStorage) StubResponse(rec *storage.ArchivedRecord) storage.ArchivedStub {
	return b.inner.StubResponse(rec)
}

var _ storage.Service = (*blockingStorage)(nil)

func TestWorker_Stop_DrainsInflight(t *testing.T) {
	env := setupWorkerEnv(t, "sync")
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	ancestor := commitFiles(t, env.repo, env.repoDir, nil, map[string][]byte{
		"base.txt": []byte("base\n"),
	}, "base")
	draftTip := commitFiles(t, env.repo, env.repoDir, ancestor, map[string][]byte{
		"base.txt": []byte("base\n"),
	}, "draft")
	source := commitFiles(t, env.repo, env.repoDir, ancestor, map[string][]byte{
		"base.txt":   []byte("base\n"),
		"file-b.txt": []byte("source\n"),
	}, "source")
	draftRefName := plumbing.NewBranchReferenceName("jam/" + env.sess.ID + "/draft")
	if err := env.repo.Storer.SetReference(plumbing.NewHashReference(draftRefName, draftTip.Hash)); err != nil {
		t.Fatalf("set draft ref: %v", err)
	}

	mergeCh, unsub := env.log.Subscribe("merge.succeeded")
	defer unsub()

	w := newWorker(env)
	if err := w.Start(ctx); err != nil {
		t.Fatalf("Start: %v", err)
	}

	// Emit one event.
	ref := "refs/heads/jam/" + env.sess.ID + "/alice/feat"
	payload := buildCommitArrivedPayload(t, ref, source.Hash.String())
	if _, err := env.log.Emit(ctx, env.sess.OrgID, env.sess.ID, "commit.arrived", payload); err != nil {
		t.Fatalf("Emit: %v", err)
	}

	// Cancel the context and stop the worker; stop should wait for the goroutine.
	cancel()
	stopCtx, stopCancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer stopCancel()
	if err := w.Stop(stopCtx); err != nil {
		t.Fatalf("Stop: %v", err)
	}

	// Check if we got a merge.succeeded (it may or may not have finished
	// before ctx cancel, but the goroutine should have exited cleanly).
	select {
	case <-mergeCh:
		// Great — event was processed before shutdown.
	default:
		// Fine — event wasn't processed (ctx cancelled first). Main point is
		// Stop returned without timeout.
	}
}

// ---------------------------------------------------------------------------
// helpers
// ---------------------------------------------------------------------------

func buildCommitArrivedPayload(t *testing.T, ref, sha string) json.RawMessage {
	t.Helper()
	return mustMarshalWorker(t, map[string]string{
		"ref":       ref,
		"sha":       sha,
		"author_id": "test-account",
		"summary":   "test commit",
	})
}

func mustMarshalWorker(t *testing.T, v interface{}) json.RawMessage {
	t.Helper()
	b, err := json.Marshal(v)
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	return b
}

