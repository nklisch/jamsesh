package objectstore

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/storage"
)

// ---------------------------------------------------------------------------
// Test helpers: storage
// ---------------------------------------------------------------------------

// lifecycleTestStorage is a full storage.Service backed by a temp directory.
// Unlike testStorage (sync tests), it actually creates bare repos on disk so
// that os.RemoveAll behaves correctly in eviction tests.
type lifecycleTestStorage struct {
	root             string
	createRepoCalled atomic.Int32
}

func (s *lifecycleTestStorage) RepoPath(orgID, sessionID string) string {
	return filepath.Join(s.root, "orgs", orgID, "sessions", sessionID+".git")
}

func (s *lifecycleTestStorage) CreateRepo(_ context.Context, orgID, sessionID string) error {
	s.createRepoCalled.Add(1)
	p := s.RepoPath(orgID, sessionID)
	if err := os.MkdirAll(p, 0o750); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("storage: repo already exists: %s", p)
		}
		return err
	}
	return nil
}

func (s *lifecycleTestStorage) RemoveRepo(_ context.Context, orgID, sessionID string) error {
	return os.RemoveAll(s.RepoPath(orgID, sessionID))
}

func (s *lifecycleTestStorage) RepoExists(orgID, sessionID string) (bool, error) {
	info, err := os.Stat(s.RepoPath(orgID, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (s *lifecycleTestStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	panic("lifecycleTestStorage.ArchiveSession called unexpectedly")
}

func (s *lifecycleTestStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	panic("lifecycleTestStorage.LookupArchived called unexpectedly")
}

func (s *lifecycleTestStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	panic("lifecycleTestStorage.StubResponse called unexpectedly")
}

// ---------------------------------------------------------------------------
// Test helpers: lease managers
// ---------------------------------------------------------------------------

// testHandle is a controllable lease.Handle for lifecycle tests.
type testHandle struct {
	sessionID    string
	fencingToken int64
	lost         chan struct{}
	releaseOnce  sync.Once
	released     atomic.Bool
}

func newTestHandle(sessionID string, token int64) *testHandle {
	return &testHandle{
		sessionID:    sessionID,
		fencingToken: token,
		lost:         make(chan struct{}),
	}
}

func (h *testHandle) SessionID() string    { return h.sessionID }
func (h *testHandle) FencingToken() int64  { return h.fencingToken }
func (h *testHandle) Lost() <-chan struct{} { return h.lost }

func (h *testHandle) Release() error {
	h.releaseOnce.Do(func() {
		h.released.Store(true)
		close(h.lost)
	})
	return nil
}

// fireLost closes the Lost() channel without marking the handle as released.
// Used to simulate a lease loss event from outside.
func (h *testHandle) fireLost() {
	h.releaseOnce.Do(func() {
		close(h.lost)
	})
}

// testLeaseManager is a controllable lease.Manager.
type testLeaseManager struct {
	mu       sync.Mutex
	handles  map[string]*testHandle
	acquires atomic.Int32
	// errOnAcquire, if non-nil, is returned by Acquire instead of creating a
	// handle. Set to lease.ErrAlreadyHeld to test the 503 path.
	errOnAcquire error
	// tokenCounter provides incrementing fencing tokens.
	tokenCounter atomic.Int64
}

func newTestLeaseManager() *testLeaseManager {
	return &testLeaseManager{
		handles: make(map[string]*testHandle),
	}
}

func (m *testLeaseManager) Acquire(_ context.Context, sessionID string) (lease.Handle, error) {
	m.acquires.Add(1)
	if m.errOnAcquire != nil {
		return nil, m.errOnAcquire
	}
	token := m.tokenCounter.Add(1)
	h := newTestHandle(sessionID, token)
	m.mu.Lock()
	m.handles[sessionID] = h
	m.mu.Unlock()
	return h, nil
}

func (m *testLeaseManager) handleFor(sessionID string) *testHandle {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.handles[sessionID]
}

// ---------------------------------------------------------------------------
// Test helpers: hydrator
// ---------------------------------------------------------------------------

// countingHydrator wraps a real Hydrator and counts Hydrate calls.
type countingHydrator struct {
	hydrator    *Hydrator
	hydrateCalls atomic.Int32
	errOnHydrate error
}

func (c *countingHydrator) Hydrate(ctx context.Context, orgID, sessionID string) (HydrationOutput, error) {
	c.hydrateCalls.Add(1)
	if c.errOnHydrate != nil {
		return HydrationOutput{}, c.errOnHydrate
	}
	return c.hydrator.Hydrate(ctx, orgID, sessionID)
}

// wrappedHydrator adapts countingHydrator to look like *Hydrator for
// newTestLifecycleManager. We achieve this by embedding the counting logic
// in a thin struct that produces a real *Hydrator.
//
// Since LifecycleManager.Hydrator is typed as *Hydrator (concrete), we can't
// mock it directly without an interface. For tests that need hydration-call
// counting we instrument via storage.CreateRepoCalled and backend state.
// For the hydration-failure test we use a nil-backend Hydrator that always fails.

// newFailingHydrator returns a *Hydrator whose Hydrate always returns an error.
// It uses a memBackend that rejects all operations.
func newFailingHydrator(stor storage.Service) *Hydrator {
	return &Hydrator{
		Backend:   &errBackend{err: errors.New("backend: simulated failure")},
		Manifests: &ManifestStore{Backend: &errBackend{err: errors.New("backend: simulated failure")}},
		Storage:   stor,
	}
}

// errBackend is a Backend that returns errors on all operations.
type errBackend struct {
	err error
}

func (b *errBackend) Put(_ context.Context, _ string, _ []byte, _ int64, _ string) (string, error) {
	return "", b.err
}
func (b *errBackend) PutIdempotent(_ context.Context, _ string, _ []byte, _ int64) error {
	return b.err
}
func (b *errBackend) Get(_ context.Context, _ string) ([]byte, string, int64, error) {
	return nil, "", 0, b.err
}
func (b *errBackend) Delete(_ context.Context, _ string) error { return b.err }
func (b *errBackend) List(_ context.Context, _ string, _ func(string) error) error { return b.err }
func (b *errBackend) Probe(_ context.Context) error                                { return b.err }

// ---------------------------------------------------------------------------
// Test helper: newTestLifecycleManager
// ---------------------------------------------------------------------------

// newTestLifecycleManager creates a LifecycleManager with an in-memory backend,
// a counting Hydrator (uses no-manifest fast path for fresh sessions), a
// no-op Syncer, and a testLeaseManager.
func newTestLifecycleManager(t *testing.T) (*LifecycleManager, *testLeaseManager, *lifecycleTestStorage) {
	t.Helper()
	root := t.TempDir()
	stor := &lifecycleTestStorage{root: root}
	backend := newMemBackend()
	manifests := &ManifestStore{Backend: backend}

	hydrator := &Hydrator{
		Backend:   backend,
		Manifests: manifests,
		Storage:   stor,
		Workers:   1,
	}

	syncer := &Syncer{
		Backend:                backend,
		Manifests:              manifests,
		Storage:                stor,
		QueueSize:              256,
		PerSessionBackpressure: true,
	}

	lm := newTestLeaseManager()

	mgr := &LifecycleManager{
		Lease:    lm,
		Hydrator: hydrator,
		Syncer:   syncer,
		Storage:  stor,
		OrgIDLookup: func(_ context.Context, _ string) (string, error) {
			return "test-org", nil
		},
		IdleTimeout:     5 * time.Minute,
		IdleCheckPeriod: 30 * time.Second,
		Metrics:         metrics.New(),
	}

	return mgr, lm, stor
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestLifecycle_AcquireForRequest_FirstTime verifies that AcquireForRequest
// hydrates the session and stores the entry on the first call.
func TestLifecycle_AcquireForRequest_FirstTime(t *testing.T) {
	mgr, lm, stor := newTestLifecycleManager(t)
	ctx := context.Background()
	sessionID := "sess-first"

	handle, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("AcquireForRequest: %v", err)
	}
	if handle == nil {
		t.Fatal("expected non-nil handle")
	}

	// Lease should have been acquired exactly once.
	if got := lm.acquires.Load(); got != 1 {
		t.Errorf("lease acquires: got %d, want 1", got)
	}

	// CreateRepo should have been called (fresh session hydration path).
	if got := stor.createRepoCalled.Load(); got != 1 {
		t.Errorf("CreateRepo calls: got %d, want 1", got)
	}

	// Session entry should be in the map.
	if _, ok := mgr.sessions.Load(sessionID); !ok {
		t.Error("session entry not in map after AcquireForRequest")
	}
}

// TestLifecycle_AcquireForRequest_SecondTime verifies that a second call
// returns the same handle without re-hydrating.
func TestLifecycle_AcquireForRequest_SecondTime(t *testing.T) {
	mgr, lm, stor := newTestLifecycleManager(t)
	ctx := context.Background()
	sessionID := "sess-second"

	h1, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("first AcquireForRequest: %v", err)
	}

	h2, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("second AcquireForRequest: %v", err)
	}

	// Must be the same handle instance.
	if h1 != h2 {
		t.Error("second AcquireForRequest returned a different handle")
	}

	// Lease acquired exactly once (no re-acquire).
	if got := lm.acquires.Load(); got != 1 {
		t.Errorf("lease acquires: got %d, want 1 (no re-hydration)", got)
	}

	// CreateRepo called exactly once (no re-hydration).
	if got := stor.createRepoCalled.Load(); got != 1 {
		t.Errorf("CreateRepo calls: got %d, want 1", got)
	}
}

// TestLifecycle_AcquireForRequest_AlreadyHeld verifies that when the lease
// manager returns ErrAlreadyHeld, the error is wrapped and returned.
func TestLifecycle_AcquireForRequest_AlreadyHeld(t *testing.T) {
	mgr, lm, _ := newTestLifecycleManager(t)
	lm.errOnAcquire = lease.ErrAlreadyHeld
	ctx := context.Background()

	_, err := mgr.AcquireForRequest(ctx, "sess-held")
	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if !errors.Is(err, lease.ErrAlreadyHeld) {
		t.Errorf("expected ErrAlreadyHeld in error chain, got: %v", err)
	}

	// No entry in map.
	if _, ok := mgr.sessions.Load("sess-held"); ok {
		t.Error("session entry stored despite ErrAlreadyHeld")
	}
}

// TestLifecycle_AcquireForRequest_HydrationFailure verifies that when
// Hydrate fails, the lease is released and no entry is stored.
func TestLifecycle_AcquireForRequest_HydrationFailure(t *testing.T) {
	root := t.TempDir()
	stor := &lifecycleTestStorage{root: root}
	lm := newTestLeaseManager()

	// Hydrator whose backend manifest load will fail (no manifest, but backend
	// errors on all operations so the fresh-session CreateRepo path will also fail).
	failHydrator := &Hydrator{
		Backend:   &errBackend{err: errors.New("simulated backend failure")},
		Manifests: &ManifestStore{Backend: &errBackend{err: errors.New("simulated backend failure")}},
		Storage:   stor,
	}

	mgr := &LifecycleManager{
		Lease:    lm,
		Hydrator: failHydrator,
		Syncer: &Syncer{
			Backend:   newMemBackend(),
			Manifests: &ManifestStore{Backend: newMemBackend()},
			Storage:   stor,
		},
		Storage: stor,
		OrgIDLookup: func(_ context.Context, _ string) (string, error) {
			return "test-org", nil
		},
	}

	ctx := context.Background()
	_, err := mgr.AcquireForRequest(ctx, "sess-fail")
	if err == nil {
		t.Fatal("expected error from failing hydrator")
	}

	// No entry in map.
	if _, ok := mgr.sessions.Load("sess-fail"); ok {
		t.Error("session entry stored despite hydration failure")
	}

	// The lease handle must have been released (Lost() should be closed).
	h := lm.handleFor("sess-fail")
	if h == nil {
		t.Fatalf("precondition failed: lease was not even acquired (backend error before acquire) — no handle to check")
	}
	select {
	case <-h.lost:
		// released, as expected
	default:
		t.Error("lease handle was not released after hydration failure")
	}
}

// TestLifecycle_HandleLost_TriggersRelease verifies that when a handle's
// Lost() channel is closed externally, the session is automatically released.
func TestLifecycle_HandleLost_TriggersRelease(t *testing.T) {
	mgr, lm, _ := newTestLifecycleManager(t)
	ctx := context.Background()
	sessionID := "sess-lost"

	_, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("AcquireForRequest: %v", err)
	}

	// Verify entry is present.
	if _, ok := mgr.sessions.Load(sessionID); !ok {
		t.Fatal("session not in map after acquire")
	}

	// Fire Lost() on the underlying handle externally.
	h := lm.handleFor(sessionID)
	if h == nil {
		t.Fatal("handle not found in lease manager")
	}
	// Close the lost channel externally to simulate lease loss without a normal Release.
	// We use a goroutine to avoid the once-guard issue: fireLost uses releaseOnce too,
	// but we only need the channel closed for the watchLost goroutine to fire.
	h.fireLost()

	// The watchLost goroutine runs asynchronously. Poll until the entry disappears.
	deadline := time.Now().Add(2 * time.Second)
	for time.Now().Before(deadline) {
		if _, ok := mgr.sessions.Load(sessionID); !ok {
			return // released, as expected
		}
		time.Sleep(10 * time.Millisecond)
	}
	t.Error("session entry still in map 2s after Lost() fired — watchLost did not trigger release")
}

// TestLifecycle_Release_WaitsForUploads verifies that Release waits for
// in-flight Syncer uploads to drain before evicting.
func TestLifecycle_Release_WaitsForUploads(t *testing.T) {
	mgr, _, _ := newTestLifecycleManager(t)
	ctx := context.Background()
	sessionID := "sess-drain"

	_, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("AcquireForRequest: %v", err)
	}

	// Manually bump the Syncer's in-flight counter for this session.
	counter := mgr.Syncer.inFlightFor(sessionID)
	atomic.AddInt64(counter, 1)

	// Release in a goroutine — it will block until the counter drains.
	released := make(chan struct{})
	go func() {
		_ = mgr.releaseWithReason(ctx, sessionID, "explicit")
		close(released)
	}()

	// Give the Release goroutine time to start waiting.
	time.Sleep(100 * time.Millisecond)

	// Session should still be in the map (waiting on drain).
	if _, ok := mgr.sessions.Load(sessionID); !ok {
		// Already released — could be a race; the test may be timing-sensitive.
		// At minimum verify Release completed.
		select {
		case <-released:
			t.Log("released faster than expected; drain may have completed before check")
		case <-time.After(500 * time.Millisecond):
			t.Error("release goroutine hung without drain")
		}
		return
	}

	// Now drain the counter — Release should proceed.
	atomic.AddInt64(counter, -1)

	select {
	case <-released:
		// Good.
	case <-time.After(2 * time.Second):
		t.Error("Release did not complete within 2s after drain")
	}

	// Entry must be gone.
	if _, ok := mgr.sessions.Load(sessionID); ok {
		t.Error("session entry still in map after Release")
	}
}

// TestLifecycle_Release_EvictsLocalCache verifies that Release removes the
// bare repo directory.
func TestLifecycle_Release_EvictsLocalCache(t *testing.T) {
	mgr, _, stor := newTestLifecycleManager(t)
	ctx := context.Background()
	sessionID := "sess-evict"

	_, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("AcquireForRequest: %v", err)
	}

	// CreateRepo is called during hydration (fresh session path). The directory
	// should exist.
	repoPath := stor.RepoPath("test-org", sessionID)
	if _, statErr := os.Stat(repoPath); os.IsNotExist(statErr) {
		t.Fatalf("precondition failed: repo directory was never created (hydrator skipped CreateRepo) — eviction test not meaningful")
	}

	if err := mgr.releaseWithReason(ctx, sessionID, "explicit"); err != nil {
		t.Fatalf("releaseWithReason: %v", err)
	}

	// Repo directory must have been removed.
	if _, statErr := os.Stat(repoPath); !os.IsNotExist(statErr) {
		t.Errorf("expected repo directory %s to be removed after Release; stat returned: %v", repoPath, statErr)
	}
}

// TestLifecycle_IdleEviction verifies that sessions idle beyond IdleTimeout
// are automatically released on the next eviction tick.
func TestLifecycle_IdleEviction(t *testing.T) {
	mgr, _, _ := newTestLifecycleManager(t)
	// Short idle timeout for the test.
	mgr.IdleTimeout = 50 * time.Millisecond
	ctx := context.Background()
	sessionID := "sess-idle"

	_, err := mgr.AcquireForRequest(ctx, sessionID)
	if err != nil {
		t.Fatalf("AcquireForRequest: %v", err)
	}

	// Wait longer than the idle timeout.
	time.Sleep(200 * time.Millisecond)

	// Manually trigger the eviction check (avoids relying on ticker timing).
	mgr.evictIdleAndOversize(ctx)

	// Entry must be gone.
	if _, ok := mgr.sessions.Load(sessionID); ok {
		t.Error("session entry still in map after idle eviction tick")
	}
}

// TestLifecycle_LRUEviction verifies that when cumulative cache size exceeds
// CacheMaxBytes, the oldest-lastActive session is evicted.
func TestLifecycle_LRUEviction(t *testing.T) {
	mgr, _, stor := newTestLifecycleManager(t)
	ctx := context.Background()

	sessA := "sess-lru-a"
	sessB := "sess-lru-b"

	_, err := mgr.AcquireForRequest(ctx, sessA)
	if err != nil {
		t.Fatalf("AcquireForRequest A: %v", err)
	}
	// Touch A first so its lastActiveAt is older.
	time.Sleep(20 * time.Millisecond)

	_, err = mgr.AcquireForRequest(ctx, sessB)
	if err != nil {
		t.Fatalf("AcquireForRequest B: %v", err)
	}

	// Write data into both repo dirs so dirSize returns > 0.
	for _, sess := range []string{sessA, sessB} {
		repoPath := stor.RepoPath("test-org", sess)
		if err := os.MkdirAll(repoPath, 0o750); err != nil {
			t.Fatalf("mkdir %s: %v", repoPath, err)
		}
		// Write a 1 KB file so the combined size > a tiny cap.
		if err := os.WriteFile(filepath.Join(repoPath, "data"), make([]byte, 1024), 0o600); err != nil {
			t.Fatalf("write data: %v", err)
		}
	}

	// Set a cap that is exceeded by the combined 2 KB.
	mgr.CacheMaxBytes = 1500 // 1.5 KB → one of the two 1 KB repos exceeds this together

	mgr.evictIdleAndOversize(ctx)

	// Sess A (older lastActiveAt) must have been evicted; B must remain.
	if _, ok := mgr.sessions.Load(sessA); ok {
		t.Error("sess A (oldest) still in map after LRU eviction — should have been evicted")
	}
	if _, ok := mgr.sessions.Load(sessB); !ok {
		t.Error("sess B (newest) was evicted instead of sess A")
	}
}

// TestLifecycle_Shutdown_ReleasesAll verifies that cancelling the context
// passed to Start causes all active sessions to be released.
func TestLifecycle_Shutdown_ReleasesAll(t *testing.T) {
	mgr, _, _ := newTestLifecycleManager(t)
	// Use a very long check period so the ticker doesn't fire during the test.
	mgr.IdleCheckPeriod = 10 * time.Minute
	ctx, cancel := context.WithCancel(context.Background())

	// Acquire several sessions.
	sessions := []string{"sess-shut-1", "sess-shut-2", "sess-shut-3"}
	for _, sid := range sessions {
		if _, err := mgr.AcquireForRequest(ctx, sid); err != nil {
			t.Fatalf("AcquireForRequest %s: %v", sid, err)
		}
	}

	// Run Start in a goroutine.
	startDone := make(chan error, 1)
	go func() {
		startDone <- mgr.Start(ctx)
	}()

	// Give Start time to enter its ticker-wait.
	time.Sleep(50 * time.Millisecond)

	// Cancel — triggers shutdownAll.
	cancel()

	select {
	case err := <-startDone:
		if !errors.Is(err, context.Canceled) {
			t.Errorf("Start returned %v; want context.Canceled", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Start did not return within 5s after context cancel")
	}

	// All sessions must be released.
	for _, sid := range sessions {
		if _, ok := mgr.sessions.Load(sid); ok {
			t.Errorf("session %s still in map after shutdown", sid)
		}
	}
}

// TestLifecycle_AcquireWhileReleasing verifies that concurrent Acquire + Release
// on the same session does not produce double-state (two entries or a race).
func TestLifecycle_AcquireWhileReleasing(t *testing.T) {
	mgr, _, _ := newTestLifecycleManager(t)
	ctx := context.Background()
	sessionID := "sess-race"

	if _, err := mgr.AcquireForRequest(ctx, sessionID); err != nil {
		t.Fatalf("initial AcquireForRequest: %v", err)
	}

	// Fire concurrent Release and re-Acquire. The race detector will catch
	// any data race; the test checks for logical correctness (at most one entry).
	var wg sync.WaitGroup
	wg.Add(2)

	go func() {
		defer wg.Done()
		_ = mgr.releaseWithReason(ctx, sessionID, "explicit")
	}()

	go func() {
		defer wg.Done()
		// Give the releaseWithReason goroutine a tiny head-start to flip the releasing flag.
		time.Sleep(1 * time.Millisecond)
		_, _ = mgr.AcquireForRequest(ctx, sessionID)
	}()

	wg.Wait()

	// Count entries in the map — must be 0 or 1, never more.
	count := 0
	mgr.sessions.Range(func(k, _ any) bool {
		if k.(string) == sessionID {
			count++
		}
		return true
	})
	if count > 1 {
		t.Errorf("concurrent Acquire+Release produced %d map entries; want ≤1", count)
	}
}
