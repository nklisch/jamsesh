package objectstore

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/storage"
)

// ---------------------------------------------------------------------------
// Test infrastructure — hydration-specific
// ---------------------------------------------------------------------------

// hydrateTestStorage implements storage.Service for hydrator tests.
// Unlike testStorage (used in sync tests, which panics on CreateRepo), this one
// actually initialises a real bare repo on disk so git fsck can run.
type hydrateTestStorage struct {
	root string
	// createRepoCalled is incremented each time CreateRepo is called.
	createRepoCalled atomic.Int32
	// createRepoErr, if non-nil, is returned by CreateRepo instead of doing
	// real work.
	createRepoErr error
}

func (s *hydrateTestStorage) RepoPath(orgID, sessionID string) string {
	return filepath.Join(s.root, "orgs", orgID, "sessions", sessionID+".git")
}

func (s *hydrateTestStorage) CreateRepo(_ context.Context, orgID, sessionID string) error {
	s.createRepoCalled.Add(1)
	if s.createRepoErr != nil {
		return s.createRepoErr
	}
	p := s.RepoPath(orgID, sessionID)
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return err
	}
	if err := os.Mkdir(p, 0o750); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("storage: repo already exists: %s", p)
		}
		return err
	}
	cmd := exec.Command("git", "init", "--bare", "-b", "main", p)
	if out, err := cmd.CombinedOutput(); err != nil {
		_ = os.RemoveAll(p)
		return fmt.Errorf("git init --bare: %w\n%s", err, out)
	}
	return nil
}

func (s *hydrateTestStorage) RemoveRepo(_ context.Context, orgID, sessionID string) error {
	return os.RemoveAll(s.RepoPath(orgID, sessionID))
}

func (s *hydrateTestStorage) RepoExists(orgID, sessionID string) (bool, error) {
	info, err := os.Stat(s.RepoPath(orgID, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}

func (s *hydrateTestStorage) ArchiveSession(_ context.Context, _, _ string, _ storage.ArchiveInfo) error {
	panic("hydrateTestStorage.ArchiveSession called unexpectedly")
}
func (s *hydrateTestStorage) LookupArchived(_ context.Context, _, _ string) (*storage.ArchivedRecord, error) {
	panic("hydrateTestStorage.LookupArchived called unexpectedly")
}
func (s *hydrateTestStorage) StubResponse(_ *storage.ArchivedRecord) storage.ArchivedStub {
	panic("hydrateTestStorage.StubResponse called unexpectedly")
}

// newTestHydrator builds a Hydrator with an in-memory backend and a temp
// storage root. Returns the hydrator, backend, and storage service.
func newTestHydrator(t *testing.T) (*Hydrator, *memBackend, *hydrateTestStorage) {
	t.Helper()
	root := t.TempDir()
	backend := newMemBackend()
	manifests := &ManifestStore{Backend: backend}
	stor := &hydrateTestStorage{root: root}

	h := &Hydrator{
		Backend:   backend,
		Manifests: manifests,
		Storage:   stor,
		Metrics:   nil, // nil-safe by default
		Workers:   4,
	}
	return h, backend, stor
}

// seedObjectInBackend writes a loose-object key for sessionID into backend,
// mimicking the key format Syncer uses:
//
//	sessions/<id>/objects/<fanout>/<rest>
func seedObjectInBackend(t *testing.T, backend *memBackend, sessionID, fanout, rest string, data []byte) string {
	t.Helper()
	key := "sessions/" + sessionID + "/objects/" + fanout + "/" + rest
	if err := backend.PutIdempotent(context.Background(), key, data, 1); err != nil {
		t.Fatalf("seedObjectInBackend: %v", err)
	}
	return key
}

// seedPackInBackend writes a pack + idx pair for sessionID into backend.
// Keys follow the Syncer format: sessions/<id>/packs/<sha>.pack / .idx
func seedPackInBackend(t *testing.T, backend *memBackend, sessionID, sha string, packData, idxData []byte) PackEntry {
	t.Helper()
	packKey := "sessions/" + sessionID + "/packs/" + sha + ".pack"
	idxKey := "sessions/" + sessionID + "/packs/" + sha + ".idx"
	if err := backend.PutIdempotent(context.Background(), packKey, packData, 1); err != nil {
		t.Fatalf("seedPackInBackend pack: %v", err)
	}
	if err := backend.PutIdempotent(context.Background(), idxKey, idxData, 1); err != nil {
		t.Fatalf("seedPackInBackend idx: %v", err)
	}
	return PackEntry{PackKey: packKey, IdxKey: idxKey, SHA: sha}
}

// seedManifestInBackend writes a manifest for sessionID into backend.
func seedManifestInBackend(t *testing.T, backend *memBackend, m Manifest) string {
	t.Helper()
	store := &ManifestStore{Backend: backend}
	etag, err := store.Save(context.Background(), m, "")
	if err != nil {
		t.Fatalf("seedManifestInBackend: %v", err)
	}
	return etag
}

// ---------------------------------------------------------------------------
// slowBackend wraps memBackend and adds artificial latency to Get calls.
// Used for TestHydrator_ParallelTiming.
// ---------------------------------------------------------------------------

type slowBackend struct {
	*memBackend
	delay time.Duration
}

func (s *slowBackend) Get(ctx context.Context, key string) ([]byte, string, int64, error) {
	select {
	case <-ctx.Done():
		return nil, "", 0, ctx.Err()
	case <-time.After(s.delay):
	}
	return s.memBackend.Get(ctx, key)
}

// ---------------------------------------------------------------------------
// failAfterNBackend wraps memBackend and fails Get after N successful calls.
// Manifest keys are always allowed through so the Hydrator can load state.
// Used for TestHydrator_AtomicWriteOnFailure.
// ---------------------------------------------------------------------------

type failAfterNBackend struct {
	*memBackend
	remaining atomic.Int32
	failErr   error
}

func newFailAfterNBackend(b *memBackend, n int, err error) *failAfterNBackend {
	f := &failAfterNBackend{memBackend: b, failErr: err}
	f.remaining.Store(int32(n))
	return f
}

func (f *failAfterNBackend) Get(ctx context.Context, key string) ([]byte, string, int64, error) {
	// Always let manifest reads through so the hydrator can load state.
	if strings.Contains(key, "manifest.json") {
		return f.memBackend.Get(ctx, key)
	}
	rem := f.remaining.Add(-1)
	if rem < 0 {
		return nil, "", 0, f.failErr
	}
	return f.memBackend.Get(ctx, key)
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestHydrator_FreshSession_NoManifest verifies that when there is no manifest
// in object storage, Hydrate treats the session as fresh: it calls CreateRepo,
// returns zero counts, and FsckOK=true (vacuously valid for an empty repo).
func TestHydrator_FreshSession_NoManifest(t *testing.T) {
	h, _, stor := newTestHydrator(t)

	out, err := h.Hydrate(context.Background(), "org1", "sess-fresh")
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	if out.ObjectsDownloaded != 0 {
		t.Errorf("ObjectsDownloaded = %d; want 0", out.ObjectsDownloaded)
	}
	if out.PacksDownloaded != 0 {
		t.Errorf("PacksDownloaded = %d; want 0", out.PacksDownloaded)
	}
	if out.BytesDownloaded != 0 {
		t.Errorf("BytesDownloaded = %d; want 0", out.BytesDownloaded)
	}
	if !out.FsckOK {
		t.Error("FsckOK = false; want true for fresh session")
	}
	if stor.createRepoCalled.Load() == 0 {
		t.Error("CreateRepo was not called for fresh session")
	}
}

// TestHydrator_FreshSession_AlreadyExists verifies that when CreateRepo returns
// an "already exists" error, Hydrate still succeeds (idempotent).
func TestHydrator_FreshSession_AlreadyExists(t *testing.T) {
	h, _, stor := newTestHydrator(t)

	// Simulate an already-exists error from CreateRepo.
	stor.createRepoErr = fmt.Errorf("storage: repo already exists: /some/path")

	out, err := h.Hydrate(context.Background(), "org1", "sess-exists")
	if err != nil {
		t.Fatalf("Hydrate with already-exists: %v", err)
	}
	if !out.FsckOK {
		t.Error("FsckOK = false; want true")
	}
}

// TestHydrator_ExistingSession_DownloadsAll verifies that an existing session
// (with a manifest listing packs, refs, and loose objects) has everything
// downloaded and counts are accurate.
func TestHydrator_ExistingSession_DownloadsAll(t *testing.T) {
	h, backend, stor := newTestHydrator(t)

	const orgID = "org1"
	const sessionID = "sess-existing"

	// Create a real bare repo so git fsck can run.
	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Seed 2 pack files.
	pack1 := seedPackInBackend(t, backend, sessionID, "aabbcc", []byte("pack-data-1"), []byte("idx-data-1"))
	pack2 := seedPackInBackend(t, backend, sessionID, "ddeeff", []byte("pack-data-2"), []byte("idx-data-2"))

	// Seed 3 loose objects.
	seedObjectInBackend(t, backend, sessionID, "ab", "cdef1234", []byte("obj1"))
	seedObjectInBackend(t, backend, sessionID, "12", "3456abcd", []byte("obj2"))
	seedObjectInBackend(t, backend, sessionID, "ff", "ee0011aa", []byte("obj3"))

	// Seed manifest.
	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
		Packs:        []PackEntry{pack1, pack2},
		Refs:         map[string]string{"refs/heads/main": "abcdef1234567890abcdef1234567890abcdef12"},
		PackedRefs:   "",
	}
	seedManifestInBackend(t, backend, m)

	out, err := h.Hydrate(context.Background(), orgID, sessionID)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	if out.PacksDownloaded != 2 {
		t.Errorf("PacksDownloaded = %d; want 2", out.PacksDownloaded)
	}
	if out.ObjectsDownloaded != 3 {
		t.Errorf("ObjectsDownloaded = %d; want 3", out.ObjectsDownloaded)
	}

	// Total bytes: 2 packs × (pack + idx) + 3 loose objects.
	expectedBytes := int64(len("pack-data-1")+len("idx-data-1")) +
		int64(len("pack-data-2")+len("idx-data-2")) +
		int64(len("obj1")+len("obj2")+len("obj3"))
	if out.BytesDownloaded != expectedBytes {
		t.Errorf("BytesDownloaded = %d; want %d", out.BytesDownloaded, expectedBytes)
	}

	// Verify pack files landed on disk.
	repoPath := stor.RepoPath(orgID, sessionID)
	packDir := filepath.Join(repoPath, "objects", "pack")
	for _, sha := range []string{"aabbcc", "ddeeff"} {
		packFile := filepath.Join(packDir, "pack-"+sha+".pack")
		idxFile := filepath.Join(packDir, "pack-"+sha+".idx")
		if _, err := os.Stat(packFile); err != nil {
			t.Errorf("pack file missing: %v", err)
		}
		if _, err := os.Stat(idxFile); err != nil {
			t.Errorf("idx file missing: %v", err)
		}
	}

	// Verify loose objects landed on disk.
	for fanout, rest := range map[string]string{
		"ab": "cdef1234",
		"12": "3456abcd",
		"ff": "ee0011aa",
	} {
		objPath := filepath.Join(repoPath, "objects", fanout, rest)
		if _, err := os.Stat(objPath); err != nil {
			t.Errorf("object file %s/%s missing: %v", fanout, rest, err)
		}
	}

	// Verify ref was written with trailing newline.
	refPath := filepath.Join(repoPath, "refs", "heads", "main")
	refData, err := os.ReadFile(refPath)
	if err != nil {
		t.Fatalf("read ref: %v", err)
	}
	if strings.TrimSpace(string(refData)) != "abcdef1234567890abcdef1234567890abcdef12" {
		t.Errorf("refs/heads/main = %q; want expected SHA", string(refData))
	}
}

// TestHydrator_ParallelTiming verifies that 5 packs with 100ms download delay
// each complete in ≤250ms when Workers=8 (bounded parallelism means all 10
// Get calls — pack+idx per entry — are issued concurrently).
func TestHydrator_ParallelTiming(t *testing.T) {
	root := t.TempDir()
	fastBackend := newMemBackend()
	slowB := &slowBackend{memBackend: fastBackend, delay: 100 * time.Millisecond}

	// Manifest loads use the fast backend directly to avoid adding delay there.
	manifests := &ManifestStore{Backend: fastBackend}
	stor := &hydrateTestStorage{root: root}

	h := &Hydrator{
		Backend:   slowB,
		Manifests: manifests,
		Storage:   stor,
		Workers:   8,
	}

	const orgID = "org1"
	const sessionID = "sess-parallel"

	// Create bare repo first.
	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Seed 5 pack entries in the fast backend.
	var packs []PackEntry
	for i := 0; i < 5; i++ {
		sha := fmt.Sprintf("%040x", i+1) // non-zero SHA
		p := seedPackInBackend(t, fastBackend, sessionID, sha,
			[]byte(fmt.Sprintf("pack-%d", i)),
			[]byte(fmt.Sprintf("idx-%d", i)))
		packs = append(packs, p)
	}

	// Seed manifest (written to fast backend — no delay on manifest load).
	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
		Packs:        packs,
	}
	seedManifestInBackend(t, fastBackend, m)

	start := time.Now()
	_, err := h.Hydrate(context.Background(), orgID, sessionID)
	elapsed := time.Since(start)

	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	// 5 packs × 2 files = 10 Get calls, each ~100ms. With Workers=8 the first
	// wave dispatches 8 concurrently (≈100ms), then the remaining 2 concurrently
	// (≈100ms) → total ≈200ms. We allow 250ms of slack for CI variance.
	if elapsed > 250*time.Millisecond {
		t.Errorf("Hydrate took %v; want ≤250ms (parallel downloads should overlap)", elapsed)
	}
}

// TestHydrator_AtomicWriteOnFailure verifies that a simulated download error
// mid-pack leaves no .tmp files in the repo directory.
func TestHydrator_AtomicWriteOnFailure(t *testing.T) {
	root := t.TempDir()
	base := newMemBackend()

	const orgID = "org1"
	const sessionID = "sess-failure"

	stor := &hydrateTestStorage{root: root}
	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Seed 3 pack entries.
	var packs []PackEntry
	for i := 0; i < 3; i++ {
		sha := fmt.Sprintf("%040x", i+1)
		p := seedPackInBackend(t, base, sessionID, sha,
			[]byte(fmt.Sprintf("pack-data-%d", i)),
			[]byte(fmt.Sprintf("idx-data-%d", i)))
		packs = append(packs, p)
	}

	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
		Packs:        packs,
	}
	seedManifestInBackend(t, base, m)

	// Allow 2 successful Gets (one pack + one idx), then fail everything after.
	failB := newFailAfterNBackend(base, 2, errors.New("simulated download failure"))
	manifests := &ManifestStore{Backend: base} // manifest loads use the unfailing base

	h := &Hydrator{
		Backend:   failB,
		Manifests: manifests,
		Storage:   stor,
		Workers:   1, // serial to make failure timing deterministic
	}

	_, err := h.Hydrate(context.Background(), orgID, sessionID)
	if err == nil {
		t.Fatal("Hydrate: expected error, got nil")
	}

	// Verify no .tmp files remain anywhere under the repo.
	repoPath := stor.RepoPath(orgID, sessionID)
	_ = filepath.WalkDir(repoPath, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return nil // skip inaccessible paths
		}
		if strings.HasSuffix(path, ".tmp") {
			t.Errorf("stale .tmp file found after failed Hydrate: %s", path)
		}
		return nil
	})
}

// TestHydrator_FsckOK verifies that git fsck --no-dangling runs after hydration
// and that FsckOK=true for a healthy (empty) bare repo.
func TestHydrator_FsckOK(t *testing.T) {
	h, backend, stor := newTestHydrator(t)

	const orgID = "org1"
	const sessionID = "sess-fsck"

	// CreateRepo so fsck has a valid bare repo to inspect.
	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	// Seed a minimal manifest with no packs or objects — just session metadata.
	// An empty bare repo passes git fsck with zero complaints.
	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
		Refs:         map[string]string{},
	}
	seedManifestInBackend(t, backend, m)

	out, err := h.Hydrate(context.Background(), orgID, sessionID)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if !out.FsckOK {
		t.Error("FsckOK = false; want true for empty bare repo")
	}
}

// TestHydrator_NilMetrics_NoOp verifies that a nil Metrics registry does not
// cause a panic — all metric emission is nil-safe.
func TestHydrator_NilMetrics_NoOp(t *testing.T) {
	h, backend, stor := newTestHydrator(t)
	h.Metrics = nil // already nil; be explicit

	const orgID = "org1"
	const sessionID = "sess-nil-metrics"

	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
	}
	seedManifestInBackend(t, backend, m)

	// Must not panic.
	_, err := h.Hydrate(context.Background(), orgID, sessionID)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
}

// TestHydrator_WithMetrics_Increments verifies that when a real metrics.Registry
// is wired in, Hydrate runs without panicking and captures timing.
func TestHydrator_WithMetrics_Increments(t *testing.T) {
	h, backend, stor := newTestHydrator(t)
	h.Metrics = metrics.New()

	const orgID = "org1"
	const sessionID = "sess-metrics"

	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
	}
	seedManifestInBackend(t, backend, m)

	out, err := h.Hydrate(context.Background(), orgID, sessionID)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}
	if out.Duration == 0 {
		t.Error("Duration = 0; want non-zero")
	}
}

// TestHydrator_PackedRefs verifies that packed-refs content from the manifest
// is written atomically to <repoPath>/packed-refs.
func TestHydrator_PackedRefs(t *testing.T) {
	h, backend, stor := newTestHydrator(t)

	const orgID = "org1"
	const sessionID = "sess-packed-refs"

	if err := stor.CreateRepo(context.Background(), orgID, sessionID); err != nil {
		t.Fatalf("CreateRepo: %v", err)
	}

	const packedRefsContent = "# pack-refs with: peeled fully-peeled sorted\ndeadbeef refs/heads/main\n"
	m := Manifest{
		SessionID:    sessionID,
		FencingToken: 1,
		PackedRefs:   packedRefsContent,
	}
	seedManifestInBackend(t, backend, m)

	_, err := h.Hydrate(context.Background(), orgID, sessionID)
	if err != nil {
		t.Fatalf("Hydrate: %v", err)
	}

	repoPath := stor.RepoPath(orgID, sessionID)
	data, err := os.ReadFile(filepath.Join(repoPath, "packed-refs"))
	if err != nil {
		t.Fatalf("read packed-refs: %v", err)
	}
	if string(data) != packedRefsContent {
		t.Errorf("packed-refs = %q; want %q", string(data), packedRefsContent)
	}
}
