// Package objectstore_test — GCS Backend integration tests.
//
// # Running the tests
//
// The tests require a live GCS bucket. Set the following environment variables:
//
//	export JAMSESH_TEST_GCS_BUCKET="jamsesh-test"
//	export JAMSESH_TEST_GCS_PREFIX="test-prefix"   # optional, defaults to ""
//	# Authentication: one of:
//	export GOOGLE_APPLICATION_CREDENTIALS="/path/to/sa.json"   # service-account key
//	# or rely on gcloud ADC / GKE Workload Identity (no env var needed)
//	go test -v ./internal/portal/storage/objectstore/...
//
// Without the env vars the suite skips cleanly with a human-readable message.
package objectstore_test

import (
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"os"
	"testing"

	"jamsesh/internal/portal/storage/objectstore"
)

// ---------------------------------------------------------------------------
// GCS harness
// ---------------------------------------------------------------------------

const (
	envGCSBucket  = "JAMSESH_TEST_GCS_BUCKET"
	envGCSPrefix  = "JAMSESH_TEST_GCS_PREFIX"
	envGCSCredFile = "JAMSESH_TEST_GCS_CRED_FILE"
)

// setupGCSBackend returns a GCS Backend or skips the test if no configuration
// is available. Each call gets a fresh random sub-prefix for test isolation.
func setupGCSBackend(t *testing.T) *testHarness {
	t.Helper()

	bucket := os.Getenv(envGCSBucket)
	if bucket == "" {
		t.Skipf(
			"GCS integration tests skipped: set %s to run (plus optional %s and GOOGLE_APPLICATION_CREDENTIALS)",
			envGCSBucket, envGCSPrefix,
		)
	}

	// Per-test random sub-prefix so concurrent runs don't collide.
	basePrefix := os.Getenv(envGCSPrefix)
	prefix := fmt.Sprintf("test-%x", rand.Int64())
	if basePrefix != "" {
		prefix = basePrefix + "/" + prefix
	}

	cfg := objectstore.GCSConfig{
		URL:             fmt.Sprintf("gs://%s/%s", bucket, prefix),
		CredentialsFile: os.Getenv(envGCSCredFile),
	}

	backend, err := objectstore.NewGCS(cfg)
	if err != nil {
		t.Fatalf("NewGCS: %v", err)
	}

	t.Cleanup(func() {
		_ = backend.List(context.Background(), "", func(key string) error {
			_ = backend.Delete(context.Background(), key)
			return nil
		})
	})

	return &testHarness{backend: backend, prefix: prefix}
}

// ---------------------------------------------------------------------------
// GCS tests — reuse the same contract as S3
// ---------------------------------------------------------------------------

func TestGCS_Put_NewObject_ReturnsETag(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	etag, err := h.backend.Put(ctx, h.key("gcs-put-new"), []byte("hello"), 1, "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if etag == "" {
		t.Error("Put returned empty ETag")
	}
}

func TestGCS_Put_UnconditionalOverwrite(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-overwrite")
	_, err := h.backend.Put(ctx, key, []byte("v1"), 1, "")
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	_, err = h.backend.Put(ctx, key, []byte("v2"), 2, "")
	if err != nil {
		t.Fatalf("Put v2: %v", err)
	}

	data, _, _, err := h.backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(data) != "v2" {
		t.Errorf("Get after overwrite = %q; want %q", data, "v2")
	}
}

func TestGCS_Put_StaleIfMatch_ReturnsErrPrecondition(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-precondition")
	etag, err := h.backend.Put(ctx, key, []byte("original"), 1, "")
	if err != nil {
		t.Fatalf("Put initial: %v", err)
	}

	// Advance the generation.
	_, err = h.backend.Put(ctx, key, []byte("updated"), 2, "")
	if err != nil {
		t.Fatalf("Put update: %v", err)
	}

	// Now use the stale ETag — must fail.
	_, err = h.backend.Put(ctx, key, []byte("conflict"), 3, etag)
	if !errors.Is(err, objectstore.ErrPrecondition) {
		t.Errorf("Put with stale ifMatch = %v; want ErrPrecondition", err)
	}
}

func TestGCS_Put_MatchingIfMatch_Succeeds(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-ifmatch-ok")
	etag, err := h.backend.Put(ctx, key, []byte("v1"), 1, "")
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	_, err = h.backend.Put(ctx, key, []byte("v2"), 2, etag)
	if err != nil {
		t.Fatalf("Put v2 with matching ifMatch: %v", err)
	}
}

func TestGCS_PutIdempotent_FirstWrite_Succeeds(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	if err := h.backend.PutIdempotent(ctx, h.key("gcs-idempotent-first"), []byte("content"), 1); err != nil {
		t.Fatalf("PutIdempotent first write: %v", err)
	}
}

func TestGCS_PutIdempotent_SameContent_Succeeds(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-idempotent-same")
	data := []byte("content-addressed blob")

	if err := h.backend.PutIdempotent(ctx, key, data, 1); err != nil {
		t.Fatalf("PutIdempotent first: %v", err)
	}
	if err := h.backend.PutIdempotent(ctx, key, data, 1); err != nil {
		t.Fatalf("PutIdempotent second (same content): %v", err)
	}
}

func TestGCS_PutIdempotent_DifferentContent_ReturnsErrAlreadyExists(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-idempotent-different")
	if err := h.backend.PutIdempotent(ctx, key, []byte("original"), 1); err != nil {
		t.Fatalf("PutIdempotent first: %v", err)
	}
	err := h.backend.PutIdempotent(ctx, key, []byte("different content"), 2)
	if !errors.Is(err, objectstore.ErrAlreadyExists) {
		t.Errorf("PutIdempotent different content = %v; want ErrAlreadyExists", err)
	}
}

func TestGCS_Get_ExistingObject_ReturnsDataETagToken(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-get-existing")
	wantData := []byte("fetched content")
	wantToken := int64(42)

	etag, err := h.backend.Put(ctx, key, wantData, wantToken, "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	gotData, gotETag, gotToken, err := h.backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if string(gotData) != string(wantData) {
		t.Errorf("Get data = %q; want %q", gotData, wantData)
	}
	if gotETag == "" {
		t.Error("Get returned empty ETag")
	}
	// GCS ETag is the generation encoded as a decimal string; Put and Get
	// should return the same generation for the same version.
	if gotETag != etag {
		t.Errorf("Get ETag = %q; Put returned %q", gotETag, etag)
	}
	if gotToken != wantToken {
		t.Errorf("Get fencingToken = %d; want %d", gotToken, wantToken)
	}
}

func TestGCS_Get_MissingObject_ReturnsErrNotFound(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	_, _, _, err := h.backend.Get(ctx, h.key(fmt.Sprintf("gcs-not-exist-%x", rand.Int64())))
	if !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get missing key = %v; want ErrNotFound", err)
	}
}

func TestGCS_Delete_Existing_Idempotent(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	key := h.key("gcs-delete-existing")
	if _, err := h.backend.Put(ctx, key, []byte("to delete"), 1, ""); err != nil {
		t.Fatalf("Put: %v", err)
	}
	if err := h.backend.Delete(ctx, key); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}
	if err := h.backend.Delete(ctx, key); err != nil {
		t.Fatalf("Delete already-deleted (must be idempotent): %v", err)
	}
	_, _, _, err := h.backend.Get(ctx, key)
	if !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get after delete = %v; want ErrNotFound", err)
	}
}

func TestGCS_Delete_Missing_Idempotent(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	err := h.backend.Delete(ctx, h.key(fmt.Sprintf("gcs-never-written-%x", rand.Int64())))
	if err != nil {
		t.Errorf("Delete non-existent key = %v; want nil", err)
	}
}

func TestGCS_List_PrefixYieldsKeys(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	type object struct {
		key  string
		data []byte
	}
	wanted := []object{
		{key: "gcs-dirs/a/1", data: []byte("1")},
		{key: "gcs-dirs/a/2", data: []byte("2")},
		{key: "gcs-dirs/a/3", data: []byte("3")},
	}
	for _, o := range wanted {
		if _, err := h.backend.Put(ctx, h.key(o.key), o.data, 1, ""); err != nil {
			t.Fatalf("Put %q: %v", o.key, err)
		}
	}
	if _, err := h.backend.Put(ctx, h.key("gcs-other/x"), []byte("x"), 1, ""); err != nil {
		t.Fatalf("Put other/x: %v", err)
	}

	var got []string
	err := h.backend.List(ctx, h.key("gcs-dirs/a/"), func(key string) error {
		got = append(got, key)
		return nil
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}
	if len(got) != len(wanted) {
		t.Fatalf("List returned %d keys; want %d: %v", len(got), len(wanted), got)
	}
	for i, o := range wanted {
		if got[i] != h.key(o.key) {
			t.Errorf("List[%d] = %q; want %q", i, got[i], h.key(o.key))
		}
	}
}

func TestGCS_List_CallbackErrorStopsIteration(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	for i := range 5 {
		key := h.key(fmt.Sprintf("gcs-stop/obj-%02d", i))
		if _, err := h.backend.Put(ctx, key, []byte("data"), 1, ""); err != nil {
			t.Fatalf("Put %q: %v", key, err)
		}
	}

	stopErr := errors.New("stop here")
	var count int
	err := h.backend.List(ctx, h.key("gcs-stop/"), func(key string) error {
		count++
		if count == 2 {
			return stopErr
		}
		return nil
	})
	if !errors.Is(err, stopErr) {
		t.Errorf("List after callback error = %v; want stopErr", err)
	}
	if count != 2 {
		t.Errorf("callback was called %d times; want 2", count)
	}
}

func TestGCS_List_Empty_ReturnsNoKeys(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	var got []string
	err := h.backend.List(ctx, h.key("gcs-no-objects-here/"), func(key string) error {
		got = append(got, key)
		return nil
	})
	if err != nil {
		t.Fatalf("List empty prefix: %v", err)
	}
	if len(got) != 0 {
		t.Errorf("List empty prefix returned keys: %v", got)
	}
}

func TestGCS_FencingTokenRoundtrips(t *testing.T) {
	h := setupGCSBackend(t)
	ctx := context.Background()

	for _, token := range []int64{0, 1, 42, 9999999999} {
		key := h.key(fmt.Sprintf("gcs-fencing-token/%d", token))
		_, err := h.backend.Put(ctx, key, []byte("data"), token, "")
		if err != nil {
			t.Fatalf("Put token=%d: %v", token, err)
		}
		_, _, got, err := h.backend.Get(ctx, key)
		if err != nil {
			t.Fatalf("Get token=%d: %v", token, err)
		}
		if got != token {
			t.Errorf("fencing token round-trip: stored %d, got %d", token, got)
		}
	}
}
