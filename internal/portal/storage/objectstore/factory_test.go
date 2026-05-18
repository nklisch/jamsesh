package objectstore_test

import (
	"strings"
	"testing"

	"jamsesh/internal/portal/storage/objectstore"
)

// TestNew_SchemeDispatch verifies that New correctly dispatches to the right
// backend constructor for each supported URL scheme. These tests are unit-level
// — they do not require any live object-storage service. We exercise the URL
// parsing and dispatch logic in isolation.
//
// The tests assert on the *shape* of errors rather than backend behaviour. Live
// backend behaviour is covered by the per-backend integration tests (s3_test.go,
// gcs_test.go, azure_test.go) which are gated on environment variables.
func TestNew_SchemeDispatch(t *testing.T) {
	t.Parallel()

	// s3:// and s3-compatible:// reach the S3 constructor. Without AWS
	// credentials or an endpoint, LoadDefaultConfig may still succeed (it
	// doesn't validate credentials eagerly), so we accept either success or a
	// provider-level error — what we must NOT get is a scheme-unknown error.
	t.Run("s3 scheme accepted", func(t *testing.T) {
		t.Parallel()
		_, err := objectstore.New("s3://my-bucket/prefix", objectstore.Config{
			Region: "us-east-1",
		})
		if err != nil {
			// The S3 constructor should not fail purely from URL parsing.
			// It may fail if AWS SDK config load fails in a severely restricted
			// environment, but that is not a scheme-dispatch error.
			if strings.Contains(err.Error(), "unknown URL scheme") {
				t.Errorf("s3:// was not accepted: %v", err)
			}
		}
	})

	t.Run("s3-compatible scheme accepted", func(t *testing.T) {
		t.Parallel()
		_, err := objectstore.New("s3-compatible://my-bucket/prefix", objectstore.Config{
			Region:      "auto",
			EndpointURL: "https://example.r2.cloudflarestorage.com",
		})
		if err != nil {
			if strings.Contains(err.Error(), "unknown URL scheme") {
				t.Errorf("s3-compatible:// was not accepted: %v", err)
			}
		}
	})

	t.Run("gs scheme accepted", func(t *testing.T) {
		t.Parallel()
		// GCS constructor calls storage.NewClient(ctx) which may fail if no
		// credentials are available. Accept any error except scheme-unknown.
		_, err := objectstore.New("gs://my-bucket/prefix", objectstore.Config{})
		if err != nil {
			if strings.Contains(err.Error(), "unknown URL scheme") {
				t.Errorf("gs:// was not accepted: %v", err)
			}
		}
	})

	t.Run("azblob scheme accepted", func(t *testing.T) {
		t.Parallel()
		// Azure constructor calls azidentity.NewDefaultAzureCredential which
		// may fail in CI without Azure credentials. Accept any error except
		// scheme-unknown.
		_, err := objectstore.New("azblob://myaccount/mycontainer/prefix", objectstore.Config{})
		if err != nil {
			if strings.Contains(err.Error(), "unknown URL scheme") {
				t.Errorf("azblob:// was not accepted: %v", err)
			}
		}
	})
}

// TestNew_UnknownScheme verifies that New rejects URL schemes it does not
// recognize with a clear error mentioning "unknown URL scheme".
func TestNew_UnknownScheme(t *testing.T) {
	t.Parallel()

	unknownURLs := []struct {
		name string
		url  string
	}{
		{"ftp", "ftp://bucket/prefix"},
		{"https", "https://bucket/prefix"},
		{"gcs", "gcs://bucket/prefix"}, // note: correct scheme is "gs"
		{"azure", "azure://account/container"},
		{"minio", "minio://bucket"},
	}

	for _, tc := range unknownURLs {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			_, err := objectstore.New(tc.url, objectstore.Config{})
			if err == nil {
				t.Fatalf("expected error for unknown scheme %q, got nil", tc.url)
			}
			if !strings.Contains(err.Error(), "unknown URL scheme") {
				t.Errorf("error for %q should mention 'unknown URL scheme', got: %v", tc.url, err)
			}
		})
	}
}

// TestNew_EmptyURL verifies that an empty URL returns a clear error.
func TestNew_EmptyURL(t *testing.T) {
	t.Parallel()
	_, err := objectstore.New("", objectstore.Config{})
	if err == nil {
		t.Fatal("expected error for empty URL, got nil")
	}
	// Should not panic; error message should be informative.
	if err.Error() == "" {
		t.Error("error message should not be empty")
	}
}

// TestNew_SchemeOnlySanity checks URLs that have a valid scheme but an
// invalid or missing bucket. These should produce errors from the underlying
// provider constructors, not from the factory dispatcher.
func TestNew_SchemeOnlySanity(t *testing.T) {
	t.Parallel()

	t.Run("s3 missing bucket", func(t *testing.T) {
		t.Parallel()
		_, err := objectstore.New("s3:///prefix-only-no-bucket", objectstore.Config{})
		if err == nil {
			t.Fatal("expected error for missing S3 bucket, got nil")
		}
		if strings.Contains(err.Error(), "unknown URL scheme") {
			t.Errorf("missing-bucket error should not be a scheme error: %v", err)
		}
	})

	t.Run("azblob missing container", func(t *testing.T) {
		t.Parallel()
		_, err := objectstore.New("azblob://account-only", objectstore.Config{})
		if err == nil {
			t.Fatal("expected error for azblob URL missing container, got nil")
		}
		if strings.Contains(err.Error(), "unknown URL scheme") {
			t.Errorf("missing-container error should not be a scheme error: %v", err)
		}
	})
}

// TestNew_S3CompatibleNormalization verifies that s3-compatible:// is correctly
// normalized to s3:// before being passed to the S3 constructor. We do this
// indirectly: if the normalization is broken, parseS3URL inside NewS3 would
// fail with a "scheme must be s3" error rather than any credential error.
func TestNew_S3CompatibleNormalization(t *testing.T) {
	t.Parallel()

	_, err := objectstore.New("s3-compatible://bucket/prefix", objectstore.Config{
		EndpointURL: "https://example.r2.cloudflarestorage.com",
	})

	// We expect the call to reach the S3 constructor successfully (URL
	// parsing passes). It may fail later on AWS config or credential issues,
	// but should NOT fail with "scheme must be s3".
	if err != nil && strings.Contains(err.Error(), "scheme must be s3") {
		t.Errorf("s3-compatible:// URL was not normalized correctly: %v", err)
	}
}
