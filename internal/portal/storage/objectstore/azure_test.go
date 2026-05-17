// Package objectstore_test — Azure Blob Storage Backend integration tests.
//
// # Running the tests
//
// The tests require a live Azure Blob Storage account (or Azurite emulator).
//
// ## Mode 1 — account key (recommended for Azurite or Azure Storage Emulator)
//
//	export JAMSESH_TEST_AZURE_URL="azblob://devstoreaccount1/jamsesh-test"
//	export JAMSESH_TEST_AZURE_ACCOUNT_KEY="Eby8vdM02xNOcqFlqUwJPLlmEtlCD..."   # Azurite default or real key
//	export JAMSESH_TEST_AZURE_SERVICE_URL="http://127.0.0.1:10000/devstoreaccount1"  # for Azurite
//	go test -v ./internal/portal/storage/objectstore/...
//
// ## Mode 2 — DefaultAzureCredential (for real Azure with Managed Identity / env vars)
//
//	export JAMSESH_TEST_AZURE_URL="azblob://myaccount/jamsesh-test"
//	# Set AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID or use managed identity
//	go test -v ./internal/portal/storage/objectstore/...
//
// Without JAMSESH_TEST_AZURE_URL the suite skips cleanly.
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
// Azure harness
// ---------------------------------------------------------------------------

const (
	envAzureURL        = "JAMSESH_TEST_AZURE_URL"
	envAzureAccountKey = "JAMSESH_TEST_AZURE_ACCOUNT_KEY"
	envAzureServiceURL = "JAMSESH_TEST_AZURE_SERVICE_URL"
)

// setupAzureBackend returns an Azure Blob Backend or skips the test if no
// configuration is available. Each call gets a fresh random sub-prefix.
func setupAzureBackend(t *testing.T) *testHarness {
	t.Helper()

	azureURL := os.Getenv(envAzureURL)
	if azureURL == "" {
		t.Skipf(
			"Azure Blob integration tests skipped: set %s to run (plus optional %s / %s)",
			envAzureURL, envAzureAccountKey, envAzureServiceURL,
		)
	}

	// Append a per-test random prefix to the configured URL so test objects
	// are isolated from concurrent runs.
	subPrefix := fmt.Sprintf("test-%x", rand.Int64())
	fullURL := azureURL + "/" + subPrefix

	cfg := objectstore.AzureBlobConfig{
		URL:        fullURL,
		AccountKey: os.Getenv(envAzureAccountKey),
		ServiceURL: os.Getenv(envAzureServiceURL),
	}

	backend, err := objectstore.NewAzureBlob(cfg)
	if err != nil {
		t.Fatalf("NewAzureBlob: %v", err)
	}

	t.Cleanup(func() {
		_ = backend.List(context.Background(), "", func(key string) error {
			_ = backend.Delete(context.Background(), key)
			return nil
		})
	})

	return &testHarness{backend: backend, prefix: subPrefix}
}

// ---------------------------------------------------------------------------
// Azure tests — same contract as S3 and GCS
// ---------------------------------------------------------------------------

func TestAzure_Put_NewObject_ReturnsETag(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	etag, err := h.backend.Put(ctx, h.key("az-put-new"), []byte("hello"), 1, "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if etag == "" {
		t.Error("Put returned empty ETag")
	}
}

func TestAzure_Put_UnconditionalOverwrite(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-overwrite")
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

func TestAzure_Put_StaleIfMatch_ReturnsErrPrecondition(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-precondition")
	etag, err := h.backend.Put(ctx, key, []byte("original"), 1, "")
	if err != nil {
		t.Fatalf("Put initial: %v", err)
	}

	// Advance the ETag.
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

func TestAzure_Put_MatchingIfMatch_Succeeds(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-ifmatch-ok")
	etag, err := h.backend.Put(ctx, key, []byte("v1"), 1, "")
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	_, err = h.backend.Put(ctx, key, []byte("v2"), 2, etag)
	if err != nil {
		t.Fatalf("Put v2 with matching ifMatch: %v", err)
	}
}

func TestAzure_PutIdempotent_FirstWrite_Succeeds(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	if err := h.backend.PutIdempotent(ctx, h.key("az-idempotent-first"), []byte("content"), 1); err != nil {
		t.Fatalf("PutIdempotent first write: %v", err)
	}
}

func TestAzure_PutIdempotent_SameContent_Succeeds(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-idempotent-same")
	data := []byte("content-addressed blob")

	if err := h.backend.PutIdempotent(ctx, key, data, 1); err != nil {
		t.Fatalf("PutIdempotent first: %v", err)
	}
	if err := h.backend.PutIdempotent(ctx, key, data, 1); err != nil {
		t.Fatalf("PutIdempotent second (same content): %v", err)
	}
}

func TestAzure_PutIdempotent_DifferentContent_ReturnsErrAlreadyExists(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-idempotent-different")
	if err := h.backend.PutIdempotent(ctx, key, []byte("original"), 1); err != nil {
		t.Fatalf("PutIdempotent first: %v", err)
	}
	err := h.backend.PutIdempotent(ctx, key, []byte("different content"), 2)
	if !errors.Is(err, objectstore.ErrAlreadyExists) {
		t.Errorf("PutIdempotent different content = %v; want ErrAlreadyExists", err)
	}
}

func TestAzure_Get_ExistingObject_ReturnsDataETagToken(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-get-existing")
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
	if gotETag != etag {
		t.Errorf("Get ETag = %q; Put returned %q", gotETag, etag)
	}
	if gotToken != wantToken {
		t.Errorf("Get fencingToken = %d; want %d", gotToken, wantToken)
	}
}

func TestAzure_Get_MissingObject_ReturnsErrNotFound(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	_, _, _, err := h.backend.Get(ctx, h.key(fmt.Sprintf("az-not-exist-%x", rand.Int64())))
	if !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get missing key = %v; want ErrNotFound", err)
	}
}

func TestAzure_Delete_Existing_Idempotent(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	key := h.key("az-delete-existing")
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

func TestAzure_Delete_Missing_Idempotent(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	err := h.backend.Delete(ctx, h.key(fmt.Sprintf("az-never-written-%x", rand.Int64())))
	if err != nil {
		t.Errorf("Delete non-existent key = %v; want nil", err)
	}
}

func TestAzure_List_PrefixYieldsKeys(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	type object struct {
		key  string
		data []byte
	}
	wanted := []object{
		{key: "az-dirs/a/1", data: []byte("1")},
		{key: "az-dirs/a/2", data: []byte("2")},
		{key: "az-dirs/a/3", data: []byte("3")},
	}
	for _, o := range wanted {
		if _, err := h.backend.Put(ctx, h.key(o.key), o.data, 1, ""); err != nil {
			t.Fatalf("Put %q: %v", o.key, err)
		}
	}
	if _, err := h.backend.Put(ctx, h.key("az-other/x"), []byte("x"), 1, ""); err != nil {
		t.Fatalf("Put other/x: %v", err)
	}

	var got []string
	err := h.backend.List(ctx, h.key("az-dirs/a/"), func(key string) error {
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

func TestAzure_List_CallbackErrorStopsIteration(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	for i := range 5 {
		key := h.key(fmt.Sprintf("az-stop/obj-%02d", i))
		if _, err := h.backend.Put(ctx, key, []byte("data"), 1, ""); err != nil {
			t.Fatalf("Put %q: %v", key, err)
		}
	}

	stopErr := errors.New("stop here")
	var count int
	err := h.backend.List(ctx, h.key("az-stop/"), func(key string) error {
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

func TestAzure_List_Empty_ReturnsNoKeys(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	var got []string
	err := h.backend.List(ctx, h.key("az-no-objects-here/"), func(key string) error {
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

func TestAzure_FencingTokenRoundtrips(t *testing.T) {
	h := setupAzureBackend(t)
	ctx := context.Background()

	for _, token := range []int64{0, 1, 42, 9999999999} {
		key := h.key(fmt.Sprintf("az-fencing-token/%d", token))
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
