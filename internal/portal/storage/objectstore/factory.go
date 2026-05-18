package objectstore

import (
	"fmt"
	"net/url"
	"strings"
)

// Config carries optional parameters that supplement the URL when constructing
// a Backend. Provider-specific authentication (AWS credentials, GCP Application
// Default Credentials, Azure identity) comes from the environment, not here.
type Config struct {
	// Region is the AWS / GCS region (e.g. "us-east-1", "us-central1").
	// Required for AWS S3; may be any non-empty string for S3-compatible
	// services (MinIO accepts any value). Not used for GCS or Azure.
	Region string

	// EndpointURL overrides the default provider endpoint. Required for
	// s3-compatible:// URLs (e.g. Cloudflare R2, MinIO). Leave empty for
	// native AWS S3, GCS, and Azure Blob.
	//
	// Example: "https://abc123.r2.cloudflarestorage.com" for Cloudflare R2.
	// Example: "http://localhost:9000" for a local MinIO instance.
	EndpointURL string

	// UsePathStyle forces path-style bucket addressing
	// (http://host/bucket/key rather than http://bucket.host/key). Required
	// for MinIO and self-hosted Ceph. Set false for AWS S3, Cloudflare R2,
	// and GCS (where it has no effect). Not used for Azure Blob.
	UsePathStyle bool

}

// New constructs a Backend from a URL and supplemental Config.
//
// Supported URL schemes:
//
//	s3://bucket/optional-prefix              → AWS S3 (default region + endpoints)
//	s3-compatible://bucket/optional-prefix   → S3-compatible endpoint (cfg.EndpointURL required)
//	gs://bucket/optional-prefix              → Google Cloud Storage (native SDK)
//	azblob://account/container/optional-prefix → Azure Blob Storage (native SDK)
//
// For s3:// and s3-compatible://, AWS credentials come from the SDK's default
// chain (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY env vars, ~/.aws/credentials,
// IAM roles for service accounts (IRSA on EKS), EC2/ECS instance metadata).
//
// For gs://, credentials come from Google Application Default Credentials
// (GOOGLE_APPLICATION_CREDENTIALS, GKE Workload Identity, gcloud CLI).
//
// For azblob://, credentials come from DefaultAzureCredential (environment
// variables, AKS Workload Identity, Azure Managed Identity, Azure CLI). To use
// a storage account key, set AZURE_STORAGE_ACCOUNT_KEY in the environment.
//
// Returns an error for unknown URL schemes or if the underlying provider
// constructor fails.
func New(rawURL string, cfg Config) (Backend, error) {
	scheme, err := parseScheme(rawURL)
	if err != nil {
		return nil, fmt.Errorf("objectstore.New: %w", err)
	}

	switch scheme {
	case "s3":
		return NewS3(S3Config{
			URL:          rawURL,
			Region:       cfg.Region,
			EndpointURL:  cfg.EndpointURL,
			UsePathStyle: cfg.UsePathStyle,
		})

	case "s3-compatible":
		// The URL scheme s3-compatible:// is not a real URI scheme — url.Parse
		// does not know about it. We normalise it to s3:// for the S3 backend
		// so bucket + prefix parsing work identically. The EndpointURL in cfg
		// is what distinguishes this from native AWS S3.
		normalized := "s3://" + strings.TrimPrefix(rawURL, "s3-compatible://")
		return NewS3(S3Config{
			URL:          normalized,
			Region:       cfg.Region,
			EndpointURL:  cfg.EndpointURL,
			UsePathStyle: cfg.UsePathStyle,
		})

	case "gs":
		return NewGCS(GCSConfig{
			URL: rawURL,
		})

	case "azblob":
		return NewAzureBlob(AzureBlobConfig{
			URL: rawURL,
		})

	default:
		return nil, fmt.Errorf("objectstore.New: unknown URL scheme %q (supported: s3, s3-compatible, gs, azblob)", scheme)
	}
}

// parseScheme extracts the URL scheme from rawURL. It handles the
// s3-compatible:// case explicitly because url.Parse treats the part before
// the first colon as the scheme, and "s3-compatible" happens to be a legal
// scheme token.
func parseScheme(rawURL string) (string, error) {
	if rawURL == "" {
		return "", fmt.Errorf("URL must not be empty")
	}

	// url.Parse handles all standard schemes plus s3-compatible since hyphens
	// are allowed in URI scheme names per RFC 3986.
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL %q: %w", rawURL, err)
	}

	scheme := strings.ToLower(u.Scheme)
	if scheme == "" {
		return "", fmt.Errorf("URL %q has no scheme (expected s3://, s3-compatible://, gs://, or azblob://)", rawURL)
	}

	return scheme, nil
}
