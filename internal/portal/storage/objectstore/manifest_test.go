package objectstore

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"sync"
	"testing"
	"time"
)

// ---------------------------------------------------------------------------
// In-memory Backend for unit tests — no S3 / MinIO required.
// ---------------------------------------------------------------------------

type memItem struct {
	data         []byte
	etag         string
	fencingToken int64
}

type memBackend struct {
	mu       sync.Mutex
	items    map[string]memItem
	nextEtag int
}

func newMemBackend() *memBackend {
	return &memBackend{items: make(map[string]memItem)}
}

func (b *memBackend) newEtag() string {
	b.nextEtag++
	return fmt.Sprintf("etag-%d", b.nextEtag)
}

func (b *memBackend) Put(_ context.Context, key string, data []byte, fencingToken int64, ifMatch string) (string, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	existing, exists := b.items[key]

	if ifMatch == "" {
		// Unconditional write — always succeeds.
	} else {
		// Conditional write — ETag must match.
		if !exists || existing.etag != ifMatch {
			return "", ErrPrecondition
		}
	}

	etag := b.newEtag()
	b.items[key] = memItem{
		data:         bytes.Clone(data),
		etag:         etag,
		fencingToken: fencingToken,
	}
	return etag, nil
}

func (b *memBackend) PutIdempotent(_ context.Context, key string, data []byte, fencingToken int64) error {
	b.mu.Lock()
	defer b.mu.Unlock()

	existing, exists := b.items[key]
	if exists {
		if bytes.Equal(existing.data, data) {
			return nil // identical content — no-op
		}
		return ErrAlreadyExists
	}

	b.items[key] = memItem{
		data:         bytes.Clone(data),
		etag:         b.newEtag(),
		fencingToken: fencingToken,
	}
	return nil
}

func (b *memBackend) Get(_ context.Context, key string) ([]byte, string, int64, error) {
	b.mu.Lock()
	defer b.mu.Unlock()

	item, ok := b.items[key]
	if !ok {
		return nil, "", 0, ErrNotFound
	}
	return bytes.Clone(item.data), item.etag, item.fencingToken, nil
}

func (b *memBackend) Delete(_ context.Context, key string) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	delete(b.items, key)
	return nil
}

func (b *memBackend) Probe(_ context.Context) error { return nil }

func (b *memBackend) List(_ context.Context, prefix string, fn func(key string) error) error {
	b.mu.Lock()
	keys := make([]string, 0, len(b.items))
	for k := range b.items {
		if len(k) >= len(prefix) && k[:len(prefix)] == prefix {
			keys = append(keys, k)
		}
	}
	b.mu.Unlock()

	// Sort for deterministic iteration (simple insertion sort is fine for
	// test-scale maps).
	for i := 1; i < len(keys); i++ {
		for j := i; j > 0 && keys[j] < keys[j-1]; j-- {
			keys[j], keys[j-1] = keys[j-1], keys[j]
		}
	}

	for _, k := range keys {
		if err := fn(k); err != nil {
			return err
		}
	}
	return nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

func newStore(b Backend) *ManifestStore {
	return &ManifestStore{Backend: b}
}

func seedManifest(t *testing.T, store *ManifestStore, m Manifest) string {
	t.Helper()
	etag, err := store.Save(context.Background(), m, "")
	if err != nil {
		t.Fatalf("seedManifest: %v", err)
	}
	return etag
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestManifestKey(t *testing.T) {
	got := ManifestKey("abc123")
	want := "sessions/abc123/manifest.json"
	if got != want {
		t.Errorf("ManifestKey(%q) = %q; want %q", "abc123", got, want)
	}
}

func TestManifestKey_EmptyID(t *testing.T) {
	// Degenerate case — valid to call, result should still be well-formed.
	got := ManifestKey("")
	want := "sessions//manifest.json"
	if got != want {
		t.Errorf("ManifestKey(%q) = %q; want %q", "", got, want)
	}
}

func TestManifestStore_Load_Missing(t *testing.T) {
	store := newStore(newMemBackend())
	m, etag, err := store.Load(context.Background(), "sess-1")
	if err != nil {
		t.Fatalf("Load missing: got err %v; want nil", err)
	}
	if etag != "" {
		t.Errorf("Load missing: got etag %q; want %q", etag, "")
	}
	// Zero-value Manifest check: key scalar fields should be zero.
	if m.SessionID != "" || m.FencingToken != 0 || m.Version != 0 || len(m.Packs) != 0 {
		t.Errorf("Load missing: got non-zero Manifest %+v; want zero-value", m)
	}
}

func TestManifestStore_Load_Existing(t *testing.T) {
	store := newStore(newMemBackend())
	want := Manifest{
		Version:      1,
		SessionID:    "sess-2",
		Packs:        []PackEntry{{PackKey: "k.pack", IdxKey: "k.idx", SHA: "aabbcc"}},
		Refs:         map[string]string{"refs/heads/main": "deadbeef"},
		PackedRefs:   "# pack-refs with: peeled fully-peeled sorted\ndeadbeef refs/heads/main\n",
		FencingToken: 7,
	}
	seedManifest(t, store, want)

	m, etag, err := store.Load(context.Background(), "sess-2")
	if err != nil {
		t.Fatalf("Load existing: %v", err)
	}
	if etag == "" {
		t.Error("Load existing: got empty ETag; want non-empty")
	}
	if m.SessionID != want.SessionID {
		t.Errorf("SessionID = %q; want %q", m.SessionID, want.SessionID)
	}
	if m.FencingToken != want.FencingToken {
		t.Errorf("FencingToken = %d; want %d", m.FencingToken, want.FencingToken)
	}
	if len(m.Packs) != 1 || m.Packs[0].SHA != "aabbcc" {
		t.Errorf("Packs = %+v; want [{SHA:aabbcc …}]", m.Packs)
	}
	if m.Refs["refs/heads/main"] != "deadbeef" {
		t.Errorf("Refs = %+v; want refs/heads/main→deadbeef", m.Refs)
	}
	if m.PackedRefs != want.PackedRefs {
		t.Errorf("PackedRefs = %q; want %q", m.PackedRefs, want.PackedRefs)
	}
}

func TestManifestStore_Save_FreshSession(t *testing.T) {
	store := newStore(newMemBackend())
	m := Manifest{
		SessionID:    "sess-fresh",
		FencingToken: 1,
	}
	etag, err := store.Save(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Save fresh session: %v", err)
	}
	if etag == "" {
		t.Error("Save fresh session: got empty ETag; want non-empty")
	}
	// Verify we can Load it back.
	loaded, _, err := store.Load(context.Background(), "sess-fresh")
	if err != nil {
		t.Fatalf("Load after Save: %v", err)
	}
	if loaded.SessionID != "sess-fresh" {
		t.Errorf("loaded.SessionID = %q; want %q", loaded.SessionID, "sess-fresh")
	}
}

func TestManifestStore_Save_FreshSession_AlreadyExists(t *testing.T) {
	store := newStore(newMemBackend())
	m := Manifest{SessionID: "sess-exists", FencingToken: 1}
	// First Save with ifMatch="" should succeed.
	_, err := store.Save(context.Background(), m, "")
	if err != nil {
		t.Fatalf("first Save: %v", err)
	}
	// Second Save with ifMatch="" should fail — manifest already exists.
	_, err = store.Save(context.Background(), m, "")
	if !errors.Is(err, ErrPrecondition) {
		t.Errorf("second Save ifMatch empty: got %v; want ErrPrecondition", err)
	}
}

func TestManifestStore_Save_MatchingETag(t *testing.T) {
	store := newStore(newMemBackend())
	m := Manifest{SessionID: "sess-match", FencingToken: 1}

	etag := seedManifest(t, store, m)

	// Advance fencing token and save with matching ETag.
	m.FencingToken = 2
	newEtag, err := store.Save(context.Background(), m, etag)
	if err != nil {
		t.Fatalf("Save with matching ETag: %v", err)
	}
	if newEtag == "" {
		t.Error("Save with matching ETag: got empty newEtag; want non-empty")
	}
	if newEtag == etag {
		t.Errorf("Save with matching ETag: newEtag == oldEtag (%q); want different", etag)
	}
}

func TestManifestStore_Save_StaleETag(t *testing.T) {
	store := newStore(newMemBackend())
	m := Manifest{SessionID: "sess-stale-etag", FencingToken: 1}

	etag := seedManifest(t, store, m)

	// Write again to advance the ETag.
	m.FencingToken = 2
	_, err := store.Save(context.Background(), m, etag)
	if err != nil {
		t.Fatalf("second Save: %v", err)
	}

	// Now try to write with the original (stale) ETag.
	m.FencingToken = 3
	_, err = store.Save(context.Background(), m, etag)
	if !errors.Is(err, ErrPrecondition) {
		t.Errorf("Save stale ETag: got %v; want ErrPrecondition", err)
	}
}

func TestManifestStore_Save_StaleFencingToken(t *testing.T) {
	store := newStore(newMemBackend())
	m := Manifest{SessionID: "sess-stale-fence", FencingToken: 5}

	etag := seedManifest(t, store, m)

	// Try to Save with a fencing token lower than the on-disk one, but with
	// the correct ETag. The fencing check should catch this before the ETag
	// check even applies.
	stale := Manifest{SessionID: "sess-stale-fence", FencingToken: 3}
	_, err := store.Save(context.Background(), stale, etag)
	if !errors.Is(err, ErrFenced) {
		t.Errorf("Save stale fencing token: got %v; want ErrFenced", err)
	}
}

func TestManifestStore_Save_SetsUpdatedAt(t *testing.T) {
	store := newStore(newMemBackend())
	before := time.Now().Add(-time.Second)

	m := Manifest{SessionID: "sess-updated-at", FencingToken: 1}
	_, err := store.Save(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, _, err := store.Load(context.Background(), "sess-updated-at")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.UpdatedAt.IsZero() {
		t.Error("UpdatedAt is zero; want it set by Save")
	}
	if loaded.UpdatedAt.Before(before) {
		t.Errorf("UpdatedAt %v is before test start %v; want it to be recent", loaded.UpdatedAt, before)
	}
}

func TestManifestStore_Save_DefaultsVersion(t *testing.T) {
	store := newStore(newMemBackend())
	// Submit with Version=0 — Save should normalise to 1.
	m := Manifest{SessionID: "sess-version", FencingToken: 1, Version: 0}
	_, err := store.Save(context.Background(), m, "")
	if err != nil {
		t.Fatalf("Save: %v", err)
	}

	loaded, _, err := store.Load(context.Background(), "sess-version")
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if loaded.Version != 1 {
		t.Errorf("Version = %d; want 1", loaded.Version)
	}
}

func TestManifest_JSONRoundTrip(t *testing.T) {
	ts := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	original := Manifest{
		Version:   1,
		SessionID: "sess-roundtrip",
		Packs: []PackEntry{
			{
				PackKey: "sessions/sess-roundtrip/packs/abc123.pack",
				IdxKey:  "sessions/sess-roundtrip/packs/abc123.idx",
				SHA:     "abc123",
			},
			{
				PackKey: "sessions/sess-roundtrip/packs/def456.pack",
				IdxKey:  "sessions/sess-roundtrip/packs/def456.idx",
				SHA:     "def456",
			},
		},
		Refs: map[string]string{
			"refs/heads/main":    "deadbeef00000000000000000000000000000000",
			"refs/heads/feature": "cafebabe00000000000000000000000000000000",
			"HEAD":               "deadbeef00000000000000000000000000000000",
		},
		PackedRefs:   "# pack-refs with: peeled fully-peeled sorted\ndeadbeef00000000000000000000000000000000 refs/heads/main\n",
		FencingToken: 42,
		UpdatedAt:    ts,
	}

	data, err := json.Marshal(original)
	if err != nil {
		t.Fatalf("Marshal: %v", err)
	}

	var decoded Manifest
	if err := json.Unmarshal(data, &decoded); err != nil {
		t.Fatalf("Unmarshal: %v", err)
	}

	// Compare field by field for a clear failure message.
	if decoded.Version != original.Version {
		t.Errorf("Version: got %d; want %d", decoded.Version, original.Version)
	}
	if decoded.SessionID != original.SessionID {
		t.Errorf("SessionID: got %q; want %q", decoded.SessionID, original.SessionID)
	}
	if decoded.FencingToken != original.FencingToken {
		t.Errorf("FencingToken: got %d; want %d", decoded.FencingToken, original.FencingToken)
	}
	if decoded.PackedRefs != original.PackedRefs {
		t.Errorf("PackedRefs: got %q; want %q", decoded.PackedRefs, original.PackedRefs)
	}
	if !decoded.UpdatedAt.Equal(original.UpdatedAt) {
		t.Errorf("UpdatedAt: got %v; want %v", decoded.UpdatedAt, original.UpdatedAt)
	}
	if len(decoded.Packs) != len(original.Packs) {
		t.Fatalf("Packs len: got %d; want %d", len(decoded.Packs), len(original.Packs))
	}
	for i, p := range decoded.Packs {
		op := original.Packs[i]
		if p.PackKey != op.PackKey || p.IdxKey != op.IdxKey || p.SHA != op.SHA {
			t.Errorf("Packs[%d]: got %+v; want %+v", i, p, op)
		}
	}
	for ref, sha := range original.Refs {
		if decoded.Refs[ref] != sha {
			t.Errorf("Refs[%q]: got %q; want %q", ref, decoded.Refs[ref], sha)
		}
	}
	if len(decoded.Refs) != len(original.Refs) {
		t.Errorf("Refs len: got %d; want %d", len(decoded.Refs), len(original.Refs))
	}
}

// TestManifestStore_Save_FencingTokenEqualOnDisk verifies that a fencing token
// equal to (not strictly less than) the on-disk token is accepted. A tie is
// NOT fenced — that would break the initial write where both sides are zero.
func TestManifestStore_Save_FencingTokenEqualOnDisk(t *testing.T) {
	store := newStore(newMemBackend())
	m := Manifest{SessionID: "sess-eq-fence", FencingToken: 5}

	etag := seedManifest(t, store, m)

	// Same fencing token — should succeed (not fenced).
	m.Refs = map[string]string{"refs/heads/main": "abc"}
	_, err := store.Save(context.Background(), m, etag)
	if err != nil {
		t.Errorf("Save equal fencing token: got %v; want nil", err)
	}
}

// TestManifestStore_Save_ErrFencedIsDistinctFromErrPrecondition verifies
// that ErrFenced and ErrPrecondition are distinct sentinel values (errors.Is
// on one should NOT match the other).
func TestManifestStore_Save_ErrFencedIsDistinctFromErrPrecondition(t *testing.T) {
	if errors.Is(ErrFenced, ErrPrecondition) {
		t.Error("ErrFenced should not match ErrPrecondition via errors.Is")
	}
	if errors.Is(ErrPrecondition, ErrFenced) {
		t.Error("ErrPrecondition should not match ErrFenced via errors.Is")
	}
}
