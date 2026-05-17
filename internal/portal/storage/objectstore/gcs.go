package objectstore

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/url"
	"strconv"
	"strings"

	"cloud.google.com/go/storage"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/iterator"
	"google.golang.org/api/option"
)

// GCSConfig configures a GCS Backend.
type GCSConfig struct {
	// URL is the object-storage location in the form gs://bucket/optional-prefix.
	// The bucket is extracted from the host component; the optional path becomes
	// the key prefix applied to every operation.
	URL string

	// CredentialsFile is an optional path to a service-account JSON key file.
	// When empty, the SDK uses Application Default Credentials (ADC), which
	// automatically resolves GKE Workload Identity, GCE service-account, and
	// GOOGLE_APPLICATION_CREDENTIALS. Prefer empty in production on GKE.
	CredentialsFile string
}

// gcsBackend is the GCS implementation of Backend.
//
// ETag semantics: GCS uses int64 generation numbers rather than ETag strings.
// We bridge by encoding the generation as a decimal string. Callers that pass
// ifMatch to Put supply the generation-as-string returned by a prior Put or
// Get call. A malformed ifMatch is treated conservatively as a precondition
// failure (unsatisfiable precondition).
type gcsBackend struct {
	client    *storage.Client
	bucket    string
	keyPrefix string // without trailing slash
}

// NewGCS constructs a GCS Backend from cfg.
//
// Authentication uses the SDK's Application Default Credentials chain:
//   - GOOGLE_APPLICATION_CREDENTIALS (path to service-account JSON key)
//   - GKE Workload Identity (metadata server — preferred for GKE production)
//   - GCE instance service account
//   - gcloud auth application-default login (local development)
//
// Override with GCSConfig.CredentialsFile for explicit key-file auth.
//
// The returned Backend is safe for concurrent use.
func NewGCS(cfg GCSConfig) (Backend, error) {
	bucket, keyPrefix, err := parseGCSURL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("objectstore/gcs: invalid URL %q: %w", cfg.URL, err)
	}

	var clientOpts []option.ClientOption
	if cfg.CredentialsFile != "" {
		clientOpts = append(clientOpts, option.WithCredentialsFile(cfg.CredentialsFile))
	}

	client, err := storage.NewClient(context.Background(), clientOpts...)
	if err != nil {
		return nil, fmt.Errorf("objectstore/gcs: create client: %w", err)
	}

	return &gcsBackend{
		client:    client,
		bucket:    bucket,
		keyPrefix: keyPrefix,
	}, nil
}

// parseGCSURL parses a URL of the form gs://bucket/optional/prefix and
// returns (bucket, prefix, nil). The prefix has no leading slash.
func parseGCSURL(rawURL string) (bucket, prefix string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "gs" {
		return "", "", fmt.Errorf("scheme must be gs, got %q", u.Scheme)
	}
	bucket = u.Host
	if bucket == "" {
		return "", "", fmt.Errorf("bucket name must not be empty")
	}
	prefix = strings.TrimPrefix(u.Path, "/")
	return bucket, prefix, nil
}

// fullKey returns the full object key for a logical key, prepending the
// configured key prefix when present.
func (b *gcsBackend) fullKey(key string) string {
	if b.keyPrefix == "" {
		return key
	}
	return b.keyPrefix + "/" + key
}

// logicalKey strips the key prefix from a full GCS object key.
func (b *gcsBackend) logicalKey(fullKey string) string {
	if b.keyPrefix == "" {
		return fullKey
	}
	prefix := b.keyPrefix + "/"
	return strings.TrimPrefix(fullKey, prefix)
}

// genToEtag encodes a GCS generation number as the ETag string used by the
// Backend interface. Callers round-trip this value opaquely.
func genToEtag(gen int64) string {
	return strconv.FormatInt(gen, 10)
}

// etagToGen decodes a generation number from the Backend ETag string.
// Returns (gen, true) on success; (0, false) if malformed.
func etagToGen(etag string) (int64, bool) {
	gen, err := strconv.ParseInt(etag, 10, 64)
	return gen, err == nil
}

// objectHandle returns an ObjectHandle for the logical key, optionally
// applying conditional write preconditions.
func (b *gcsBackend) objectHandle(key string) *storage.ObjectHandle {
	return b.client.Bucket(b.bucket).Object(b.fullKey(key))
}

// Put implements Backend.Put.
//
// GCS uses generation numbers for CAS rather than ETags. We encode the
// generation as a decimal string so the Backend interface is satisfied.
// An empty ifMatch performs an unconditional write (no precondition).
// A non-empty ifMatch is decoded as a generation; if decoding fails, we return
// ErrPrecondition (the precondition is unsatisfiable rather than wrong).
func (b *gcsBackend) Put(ctx context.Context, key string, data []byte, fencingToken int64, ifMatch string) (string, error) {
	obj := b.objectHandle(key)

	if ifMatch != "" {
		gen, ok := etagToGen(ifMatch)
		if !ok {
			// Malformed ETag cannot possibly match — treat as precondition failure.
			return "", ErrPrecondition
		}
		obj = obj.If(storage.Conditions{GenerationMatch: gen})
	}

	wc := obj.NewWriter(ctx)
	wc.Metadata = map[string]string{
		metaKeyFencingToken: strconv.FormatInt(fencingToken, 10),
	}

	if _, err := wc.Write(data); err != nil {
		_ = wc.Close()
		return "", mapGCSError(err)
	}
	if err := wc.Close(); err != nil {
		return "", mapGCSError(err)
	}

	attrs := wc.Attrs()
	if attrs == nil {
		return "", fmt.Errorf("objectstore/gcs: Put: writer returned nil attrs")
	}
	return genToEtag(attrs.Generation), nil
}

// PutIdempotent implements Backend.PutIdempotent.
//
// GCS supports create-only semantics via storage.Conditions{DoesNotExist: true}.
// On a 412 precondition failure (object already exists), we fetch the current
// content and compare bytes. Returns nil if contents match; ErrAlreadyExists
// if they differ.
func (b *gcsBackend) PutIdempotent(ctx context.Context, key string, data []byte, fencingToken int64) error {
	obj := b.objectHandle(key).If(storage.Conditions{DoesNotExist: true})
	wc := obj.NewWriter(ctx)
	wc.Metadata = map[string]string{
		metaKeyFencingToken: strconv.FormatInt(fencingToken, 10),
	}

	if _, err := wc.Write(data); err != nil {
		_ = wc.Close()
		return mapGCSError(err)
	}
	if err := wc.Close(); err != nil {
		if !isGCSPreconditionFailed(err) {
			return mapGCSError(err)
		}
		// Object already exists — compare content.
		existing, _, _, getErr := b.Get(ctx, key)
		if getErr != nil {
			return fmt.Errorf("objectstore/gcs: PutIdempotent: read existing: %w", getErr)
		}
		if bytes.Equal(existing, data) {
			return nil // idempotent success
		}
		return ErrAlreadyExists
	}
	return nil
}

// Get implements Backend.Get.
//
// GCS metadata is returned in ObjectAttrs.Metadata (map[string]string).
// The generation is returned as the ETag string.
func (b *gcsBackend) Get(ctx context.Context, key string) ([]byte, string, int64, error) {
	obj := b.objectHandle(key)

	// Fetch attrs for the ETag (generation) and metadata before reading the body.
	// GCS's NewReader does not expose generation; we need Attrs separately.
	attrs, err := obj.Attrs(ctx)
	if err != nil {
		return nil, "", 0, mapGCSError(err)
	}

	rc, err := obj.NewReader(ctx)
	if err != nil {
		return nil, "", 0, mapGCSError(err)
	}
	defer rc.Close()

	data, err := io.ReadAll(rc)
	if err != nil {
		return nil, "", 0, fmt.Errorf("objectstore/gcs: read body: %w", err)
	}

	etag := genToEtag(attrs.Generation)

	var fencingToken int64
	if tokenStr, ok := attrs.Metadata[metaKeyFencingToken]; ok && tokenStr != "" {
		fencingToken, err = strconv.ParseInt(tokenStr, 10, 64)
		if err != nil {
			fencingToken = 0 // malformed metadata is not fatal
		}
	}

	return data, etag, fencingToken, nil
}

// Delete implements Backend.Delete.
//
// GCS returns a 404 error when deleting a non-existent object. We map that
// to nil to satisfy the idempotent contract.
func (b *gcsBackend) Delete(ctx context.Context, key string) error {
	err := b.objectHandle(key).Delete(ctx)
	if err == nil {
		return nil
	}
	// Map 404 (not found) to nil — deletion is idempotent.
	var gErr *googleapi.Error
	if errors.As(err, &gErr) && gErr.Code == 404 {
		return nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return nil
	}
	return fmt.Errorf("objectstore/gcs: delete %q: %w", key, err)
}

// List implements Backend.List.
//
// GCS object listing is eventually consistent for new objects, but the list
// operation itself is strongly consistent per the GCS documentation for
// buckets without the "soft delete" feature enabled. Keys are delivered in
// UTF-8 binary order (lexicographic), matching the S3 guarantee.
func (b *gcsBackend) List(ctx context.Context, prefix string, fn func(key string) error) error {
	fullPrefix := b.fullKey(prefix)
	query := &storage.Query{Prefix: fullPrefix}

	it := b.client.Bucket(b.bucket).Objects(ctx, query)
	for {
		attrs, err := it.Next()
		if errors.Is(err, iterator.Done) {
			break
		}
		if err != nil {
			return fmt.Errorf("objectstore/gcs: list: %w", mapGCSError(err))
		}
		logKey := b.logicalKey(attrs.Name)
		if err := fn(logKey); err != nil {
			return err // caller-requested early exit
		}
	}
	return nil
}

// mapGCSError converts GCS / googleapi errors to objectstore sentinel errors.
func mapGCSError(err error) error {
	if err == nil {
		return nil
	}
	if errors.Is(err, storage.ErrObjectNotExist) {
		return ErrNotFound
	}
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case 404:
			return ErrNotFound
		case 412:
			return ErrPrecondition
		}
	}
	return err
}

// isGCSPreconditionFailed reports whether err is a 412 Precondition Failed
// from GCS (object already exists, DoesNotExist precondition violated).
func isGCSPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		return gErr.Code == 412
	}
	return false
}
