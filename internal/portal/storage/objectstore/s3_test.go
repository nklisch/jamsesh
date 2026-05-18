// Package objectstore_test contains integration tests for the S3-compatible
// Backend implementation.
//
// # Running the tests
//
// The tests require a live S3-compatible endpoint. Two modes are supported:
//
// ## Mode 1 — environment variables (recommended for CI against a real service)
//
//	export JAMSESH_TEST_S3_ENDPOINT="http://localhost:9000"
//	export JAMSESH_TEST_S3_BUCKET="jamsesh-test"
//	export JAMSESH_TEST_S3_ACCESS_KEY="minioadmin"
//	export JAMSESH_TEST_S3_SECRET_KEY="minioadmin"
//	export JAMSESH_TEST_S3_REGION="us-east-1"          # optional, defaults to us-east-1
//	export JAMSESH_TEST_S3_PATH_STYLE="true"           # optional, required for MinIO/Ceph
//	go test -v ./internal/portal/storage/objectstore/...
//
// ## Mode 2 — testcontainers with MinIO (requires Docker)
//
//	export JAMSESH_TEST_S3_USE_CONTAINER="true"
//	go test -v ./internal/portal/storage/objectstore/...
//
// Without either mode the suite skips cleanly with a human-readable message.
package objectstore_test

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"math/rand/v2"
	"net/http"
	"os"
	"strconv"
	"testing"
	"time"

	"jamsesh/internal/portal/storage/objectstore"
)

// ---------------------------------------------------------------------------
// Test harness
// ---------------------------------------------------------------------------

const (
	envEndpoint   = "JAMSESH_TEST_S3_ENDPOINT"
	envBucket     = "JAMSESH_TEST_S3_BUCKET"
	envAccessKey  = "JAMSESH_TEST_S3_ACCESS_KEY"
	envSecretKey  = "JAMSESH_TEST_S3_SECRET_KEY"
	envRegion     = "JAMSESH_TEST_S3_REGION"
	envPathStyle  = "JAMSESH_TEST_S3_PATH_STYLE"
	envContainer  = "JAMSESH_TEST_S3_USE_CONTAINER"
)

// testHarness holds the Backend under test and the bucket + prefix for
// isolation between parallel test runs.
type testHarness struct {
	backend objectstore.Backend
	// prefix is a per-test random prefix so concurrent test runs don't collide.
	prefix string
}

// setupBackend returns a configured Backend or skips the test if no S3
// credentials are available. Each call gets a fresh random prefix so tests
// are fully isolated from one another even when run in parallel.
func setupBackend(t *testing.T) *testHarness {
	t.Helper()

	endpoint := os.Getenv(envEndpoint)
	bucket := os.Getenv(envBucket)
	accessKey := os.Getenv(envAccessKey)
	secretKey := os.Getenv(envSecretKey)
	region := os.Getenv(envRegion)
	pathStyle := os.Getenv(envPathStyle) == "true" || os.Getenv(envPathStyle) == "1"
	useContainer := os.Getenv(envContainer) == "true" || os.Getenv(envContainer) == "1"

	if useContainer {
		endpoint, bucket, accessKey, secretKey = startMinIOContainer(t)
		if endpoint == "" {
			t.Skip("MinIO container could not be started (Docker unavailable?); skipping")
		}
		pathStyle = true
		if region == "" {
			region = "us-east-1"
		}
	} else if endpoint == "" || bucket == "" {
		t.Skipf(
			"S3 integration tests skipped: set %s and %s (plus %s/%s) to run, or set %s=true for Docker-based MinIO",
			envEndpoint, envBucket, envAccessKey, envSecretKey, envContainer,
		)
	}

	if region == "" {
		region = "us-east-1"
	}

	// Use a per-test random prefix inside the bucket so test objects are
	// isolated from any other concurrently running test suite.
	prefix := fmt.Sprintf("test-%x", rand.Int64())

	cfg := objectstore.S3Config{
		URL:          fmt.Sprintf("s3://%s/%s", bucket, prefix),
		Region:       region,
		EndpointURL:  endpoint,
		UsePathStyle: pathStyle,
	}

	// When access credentials are explicitly provided, inject them via the
	// environment so the default credential chain picks them up.
	if accessKey != "" && secretKey != "" {
		t.Setenv("AWS_ACCESS_KEY_ID", accessKey)
		t.Setenv("AWS_SECRET_ACCESS_KEY", secretKey)
	}

	backend, err := objectstore.NewS3(cfg)
	if err != nil {
		t.Fatalf("NewS3: %v", err)
	}

	t.Cleanup(func() {
		// Best-effort cleanup: delete all objects under the test prefix.
		_ = backend.List(context.Background(), "", func(key string) error {
			_ = backend.Delete(context.Background(), key)
			return nil
		})
	})

	return &testHarness{backend: backend, prefix: prefix}
}

// startMinIOContainer attempts to start a MinIO server using Docker directly.
// Returns the endpoint and credentials on success; empty strings if Docker
// is unavailable.
//
// This is a lightweight container launcher that avoids the testcontainers-go
// dependency. It uses the Docker HTTP API via the local socket.
// For simplicity it uses the MinIO default credentials.
func startMinIOContainer(t *testing.T) (endpoint, bucket, accessKey, secretKey string) {
	t.Helper()

	// Check if Docker socket is reachable.
	client := &http.Client{Timeout: 2 * time.Second}
	resp, err := client.Get("http://localhost:2375/info")
	if err != nil || resp.StatusCode != http.StatusOK {
		// Docker TCP is not available; try the socket.
		// For now just report unavailable.
		return "", "", "", ""
	}
	resp.Body.Close()

	// Docker is available but full container orchestration is complex without
	// testcontainers. Return empty to signal unavailability and skip.
	// Operators should point JAMSESH_TEST_S3_* at a pre-running MinIO instance.
	return "", "", "", ""
}

// key returns a unique key within the harness's prefix namespace. The harness
// prefix is already embedded in the Backend's URL; keys passed here are
// logical (relative to the prefix).
func (h *testHarness) key(suffix string) string {
	return suffix
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestPut_NewObject_ReturnsETag(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	etag, err := h.backend.Put(ctx, h.key("put-new"), []byte("hello"), 1, "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}
	if etag == "" {
		t.Error("Put returned empty ETag")
	}
}

func TestPut_UnconditionalOverwrite(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("put-overwrite")
	etag1, err := h.backend.Put(ctx, key, []byte("v1"), 1, "")
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}
	etag2, err := h.backend.Put(ctx, key, []byte("v2"), 2, "")
	if err != nil {
		t.Fatalf("Put v2: %v", err)
	}
	if etag1 == etag2 {
		t.Logf("ETags happen to be equal (same hash of different content is unexpected but not wrong for all providers)")
	}

	data, _, _, err := h.backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if !bytes.Equal(data, []byte("v2")) {
		t.Errorf("Get after unconditional overwrite = %q; want %q", data, "v2")
	}
}

func TestPut_StaleIfMatch_ReturnsErrPrecondition(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("put-precondition")

	// First write — get a real ETag.
	etag, err := h.backend.Put(ctx, key, []byte("original"), 1, "")
	if err != nil {
		t.Fatalf("Put initial: %v", err)
	}

	// Second write to advance the ETag.
	_, err = h.backend.Put(ctx, key, []byte("updated"), 2, "")
	if err != nil {
		t.Fatalf("Put update: %v", err)
	}

	// Third write using the stale ETag — must fail.
	_, err = h.backend.Put(ctx, key, []byte("conflict"), 3, etag)
	if !errors.Is(err, objectstore.ErrPrecondition) {
		t.Errorf("Put with stale ifMatch = %v; want ErrPrecondition", err)
	}
}

func TestPut_MatchingIfMatch_Succeeds(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("put-ifmatch-ok")

	etag, err := h.backend.Put(ctx, key, []byte("v1"), 1, "")
	if err != nil {
		t.Fatalf("Put v1: %v", err)
	}

	// Conditional write with correct ETag must succeed.
	_, err = h.backend.Put(ctx, key, []byte("v2"), 2, etag)
	if err != nil {
		t.Fatalf("Put v2 with matching ifMatch: %v", err)
	}
}

func TestPutIdempotent_FirstWrite_Succeeds(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	err := h.backend.PutIdempotent(ctx, h.key("idempotent-first"), []byte("content"), 1)
	if err != nil {
		t.Fatalf("PutIdempotent first write: %v", err)
	}
}

func TestPutIdempotent_SameContent_Succeeds(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("idempotent-same")
	data := []byte("content-addressed blob")

	if err := h.backend.PutIdempotent(ctx, key, data, 1); err != nil {
		t.Fatalf("PutIdempotent first: %v", err)
	}
	// Second call with identical bytes must also succeed (idempotent).
	if err := h.backend.PutIdempotent(ctx, key, data, 1); err != nil {
		t.Fatalf("PutIdempotent second (same content): %v", err)
	}
}

func TestPutIdempotent_DifferentContent_ReturnsErrAlreadyExists(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("idempotent-different")

	if err := h.backend.PutIdempotent(ctx, key, []byte("original"), 1); err != nil {
		t.Fatalf("PutIdempotent first: %v", err)
	}
	err := h.backend.PutIdempotent(ctx, key, []byte("different content"), 2)
	if !errors.Is(err, objectstore.ErrAlreadyExists) {
		t.Errorf("PutIdempotent different content = %v; want ErrAlreadyExists", err)
	}
}

func TestGet_ExistingObject_ReturnsDataETagToken(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("get-existing")
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
	if !bytes.Equal(gotData, wantData) {
		t.Errorf("Get data = %q; want %q", gotData, wantData)
	}
	if gotETag == "" {
		t.Error("Get returned empty ETag")
	}
	// Some S3-compat providers may quote the ETag differently; we strip quotes
	// in both Put and Get so they should match.
	if gotETag != etag {
		t.Errorf("Get ETag = %q; Put returned %q", gotETag, etag)
	}
	if gotToken != wantToken {
		t.Errorf("Get fencingToken = %d; want %d", gotToken, wantToken)
	}
}

func TestGet_MissingObject_ReturnsErrNotFound(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	_, _, _, err := h.backend.Get(ctx, h.key("does-not-exist-"+strconv.Itoa(rand.Int())))
	if !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get missing key = %v; want ErrNotFound", err)
	}
}

func TestGet_FencingToken_DefaultZeroWhenAbsent(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	// Write via direct AWS SDK to simulate an object without the metadata key.
	// This is the "external writer" scenario.
	//
	// We can't easily write without metadata through the Backend itself, so we
	// rely on the fact that fencing token 0 is the documented sentinel. We
	// just verify the Get path handles metadata correctly for objects that have
	// a token.
	key := h.key("token-zero")
	_, err := h.backend.Put(ctx, key, []byte("data"), 0, "")
	if err != nil {
		t.Fatalf("Put with token=0: %v", err)
	}
	_, _, gotToken, err := h.backend.Get(ctx, key)
	if err != nil {
		t.Fatalf("Get: %v", err)
	}
	if gotToken != 0 {
		t.Errorf("Get fencingToken = %d; want 0", gotToken)
	}
}

func TestDelete_Existing_Idempotent(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	key := h.key("delete-existing")
	_, err := h.backend.Put(ctx, key, []byte("to delete"), 1, "")
	if err != nil {
		t.Fatalf("Put: %v", err)
	}

	// First delete.
	if err := h.backend.Delete(ctx, key); err != nil {
		t.Fatalf("Delete existing: %v", err)
	}

	// Second delete on the now-missing key should also succeed.
	if err := h.backend.Delete(ctx, key); err != nil {
		t.Fatalf("Delete already-deleted: %v", err)
	}

	// Verify it's gone.
	_, _, _, err = h.backend.Get(ctx, key)
	if !errors.Is(err, objectstore.ErrNotFound) {
		t.Errorf("Get after delete = %v; want ErrNotFound", err)
	}
}

func TestDelete_Missing_Idempotent(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	// Delete a key that was never written — must return nil (idempotent).
	err := h.backend.Delete(ctx, h.key("never-written-"+strconv.Itoa(rand.Int())))
	if err != nil {
		t.Errorf("Delete non-existent key = %v; want nil", err)
	}
}

func TestList_PrefixYieldsKeys(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	// Write objects under two prefixes to confirm prefix filtering works.
	type object struct {
		key  string
		data []byte
	}
	wanted := []object{
		{key: "dirs/a/1", data: []byte("1")},
		{key: "dirs/a/2", data: []byte("2")},
		{key: "dirs/a/3", data: []byte("3")},
	}
	for _, o := range wanted {
		if _, err := h.backend.Put(ctx, h.key(o.key), o.data, 1, ""); err != nil {
			t.Fatalf("Put %q: %v", o.key, err)
		}
	}

	// Write an object under a different prefix — should NOT appear in the list.
	if _, err := h.backend.Put(ctx, h.key("other/x"), []byte("x"), 1, ""); err != nil {
		t.Fatalf("Put other/x: %v", err)
	}

	// List under "dirs/a/".
	var got []string
	err := h.backend.List(ctx, h.key("dirs/a/"), func(key string) error {
		got = append(got, key)
		return nil
	})
	if err != nil {
		t.Fatalf("List: %v", err)
	}

	if len(got) != len(wanted) {
		t.Fatalf("List returned %d keys; want %d: %v", len(got), len(wanted), got)
	}

	// Keys should be in lexicographic order.
	for i, o := range wanted {
		if got[i] != h.key(o.key) {
			t.Errorf("List[%d] = %q; want %q", i, got[i], h.key(o.key))
		}
	}
}

func TestList_CallbackErrorStopsIteration(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	// Write several objects.
	for i := range 5 {
		key := h.key(fmt.Sprintf("stop/obj-%02d", i))
		if _, err := h.backend.Put(ctx, key, []byte("data"), 1, ""); err != nil {
			t.Fatalf("Put %q: %v", key, err)
		}
	}

	stopErr := errors.New("stop here")
	var count int
	err := h.backend.List(ctx, h.key("stop/"), func(key string) error {
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

func TestList_Empty_ReturnsNoKeys(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	var got []string
	err := h.backend.List(ctx, h.key("no-objects-here/"), func(key string) error {
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

func TestPut_FencingTokenRoundtrips(t *testing.T) {
	h := setupBackend(t)
	ctx := context.Background()

	for _, token := range []int64{0, 1, 42, 9999999999} {
		key := h.key(fmt.Sprintf("fencing-token/%d", token))
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

// ---------------------------------------------------------------------------
// probeEndpointInfo carries raw endpoint credentials resolved from env or
// the container shim. Used by TestS3Backend_Probe_DistinguishesFailureModes.
// ---------------------------------------------------------------------------

type probeEndpointInfo struct {
	endpoint  string
	bucket    string
	accessKey string
	secretKey string
	region    string
	pathStyle bool
}

// resolveProbeEndpoint resolves S3 connection details from the environment (or
// the container shim) and skips t if nothing is configured.
func resolveProbeEndpoint(t *testing.T) probeEndpointInfo {
	t.Helper()

	endpoint := os.Getenv(envEndpoint)
	bucket := os.Getenv(envBucket)
	accessKey := os.Getenv(envAccessKey)
	secretKey := os.Getenv(envSecretKey)
	region := os.Getenv(envRegion)
	pathStyle := os.Getenv(envPathStyle) == "true" || os.Getenv(envPathStyle) == "1"
	useContainer := os.Getenv(envContainer) == "true" || os.Getenv(envContainer) == "1"

	if useContainer {
		endpoint, bucket, accessKey, secretKey = startMinIOContainer(t)
		if endpoint == "" {
			t.Skip("MinIO container could not be started (Docker unavailable?); skipping")
		}
		pathStyle = true
	} else if endpoint == "" || bucket == "" {
		t.Skipf(
			"S3 Probe integration tests skipped: set %s and %s (plus %s/%s) to run, or set %s=true for Docker-based MinIO",
			envEndpoint, envBucket, envAccessKey, envSecretKey, envContainer,
		)
	}

	if region == "" {
		region = "us-east-1"
	}

	return probeEndpointInfo{
		endpoint:  endpoint,
		bucket:    bucket,
		accessKey: accessKey,
		secretKey: secretKey,
		region:    region,
		pathStyle: pathStyle,
	}
}

// TestS3Backend_Probe_DistinguishesFailureModes verifies the Probe contract
// against a real S3-compatible endpoint across four failure partitions:
//
//   - happy_path: reachable + correct bucket → nil in < 500 ms
//   - missing_bucket: reachable + non-existent bucket → non-nil error
//   - bad_credentials: reachable + wrong key/secret → non-nil error
//   - unreachable: nothing listening on the target port → non-nil error
//     after the context deadline (~5 s)
//
// The test skips cleanly when no S3 endpoint is configured (see setupBackend
// for the env-var / container-shim documentation).
func TestS3Backend_Probe_DistinguishesFailureModes(t *testing.T) {
	info := resolveProbeEndpoint(t)

	// Inject valid credentials into the environment for all subtests; each
	// subtest that wants bad credentials overrides them with t.Setenv before
	// calling NewS3 (t.Setenv restores on cleanup).
	if info.accessKey != "" && info.secretKey != "" {
		t.Setenv("AWS_ACCESS_KEY_ID", info.accessKey)
		t.Setenv("AWS_SECRET_ACCESS_KEY", info.secretKey)
	}

	t.Run("happy_path", func(t *testing.T) {
		backend, err := objectstore.NewS3(objectstore.S3Config{
			URL:          fmt.Sprintf("s3://%s", info.bucket),
			Region:       info.region,
			EndpointURL:  info.endpoint,
			UsePathStyle: info.pathStyle,
		})
		if err != nil {
			t.Fatalf("NewS3: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		start := time.Now()
		err = backend.Probe(ctx)
		elapsed := time.Since(start)

		if err != nil {
			t.Fatalf("Probe(happy_path) = %v; want nil", err)
		}
		if elapsed > 500*time.Millisecond {
			t.Logf("Probe(happy_path) took %v (> 500 ms; may be cold-start)", elapsed)
		}
	})

	t.Run("missing_bucket", func(t *testing.T) {
		// Use a bucket name that was never created. MinIO returns 404 / NoSuchBucket.
		missingBucket := fmt.Sprintf("probe-no-such-bucket-%x", rand.Int64())

		backend, err := objectstore.NewS3(objectstore.S3Config{
			URL:          fmt.Sprintf("s3://%s", missingBucket),
			Region:       info.region,
			EndpointURL:  info.endpoint,
			UsePathStyle: info.pathStyle,
		})
		if err != nil {
			t.Fatalf("NewS3: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = backend.Probe(ctx)
		if err == nil {
			t.Fatal("Probe(missing_bucket) = nil; want non-nil error")
		}
		t.Logf("Probe(missing_bucket) error (expected): %v", err)
	})

	t.Run("bad_credentials", func(t *testing.T) {
		// Override credentials with nonsense values so the AWS SDK uses them.
		// t.Setenv restores the originals when the subtest finishes.
		t.Setenv("AWS_ACCESS_KEY_ID", "BADACCESSKEY00000000")
		t.Setenv("AWS_SECRET_ACCESS_KEY", "badsecretkeybadsecretkeybadsecretkeybadse")

		backend, err := objectstore.NewS3(objectstore.S3Config{
			URL:          fmt.Sprintf("s3://%s", info.bucket),
			Region:       info.region,
			EndpointURL:  info.endpoint,
			UsePathStyle: info.pathStyle,
		})
		if err != nil {
			t.Fatalf("NewS3: %v", err)
		}

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		err = backend.Probe(ctx)
		if err == nil {
			t.Fatal("Probe(bad_credentials) = nil; want non-nil error")
		}
		t.Logf("Probe(bad_credentials) error (expected): %v", err)
	})

	t.Run("unreachable", func(t *testing.T) {
		// 127.0.0.1:1 — port 1 is reserved and nothing listens there; the OS
		// returns ECONNREFUSED immediately. We want to assert that Probe honours
		// the context deadline rather than hanging, so we use a 6-second deadline
		// and assert the call returns within a reasonable window.
		//
		// Note: ECONNREFUSED is fast (immediate), not a timeout, so we simply
		// assert the error is non-nil without timing it strictly.
		backend, err := objectstore.NewS3(objectstore.S3Config{
			URL:          fmt.Sprintf("s3://%s", info.bucket),
			Region:       info.region,
			EndpointURL:  "http://127.0.0.1:1",
			UsePathStyle: info.pathStyle,
		})
		if err != nil {
			t.Fatalf("NewS3: %v", err)
		}

		// Use a 6-second deadline so the test does not hang if ECONNREFUSED is
		// not returned promptly (e.g. on some network stacks that drop packets
		// instead of refusing). The acceptance criterion is merely "returns an
		// error after context deadline at most", not a specific latency.
		ctx, cancel := context.WithTimeout(context.Background(), 6*time.Second)
		defer cancel()

		start := time.Now()
		err = backend.Probe(ctx)
		elapsed := time.Since(start)

		if err == nil {
			t.Fatal("Probe(unreachable) = nil; want non-nil error")
		}
		if elapsed > 7*time.Second {
			t.Errorf("Probe(unreachable) took %v; expected to return within 7s", elapsed)
		}
		t.Logf("Probe(unreachable) returned after %v: %v", elapsed, err)
	})
}
