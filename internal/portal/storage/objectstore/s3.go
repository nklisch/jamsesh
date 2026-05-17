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

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/s3"
	s3types "github.com/aws/aws-sdk-go-v2/service/s3/types"
	smithy "github.com/aws/smithy-go"
)

const (
	// metaKeyFencingToken is the object metadata key used to store the fencing
	// token alongside every uploaded object. AWS S3 normalises user-defined
	// metadata keys to lowercase, so we store in lowercase already.
	metaKeyFencingToken = "jamsesh-fencing-token"
)

// S3Config configures an S3-compatible Backend.
type S3Config struct {
	// URL is the object-storage location in the form s3://bucket/optional-prefix.
	// The bucket is extracted from the host component; the optional path becomes
	// the key prefix applied to every operation (keys passed to the Backend are
	// automatically prefixed and stripped on return).
	URL string

	// Region is the AWS region (e.g. "us-east-1"). Required for AWS S3;
	// may be any non-empty string for S3-compatible services like MinIO.
	Region string

	// EndpointURL overrides the default AWS endpoint. Set this for
	// Cloudflare R2, Backblaze B2, MinIO, self-hosted Ceph, or any other
	// S3-compatible service. Leave empty to target AWS S3.
	// Example: "http://localhost:9000" for a local MinIO instance.
	EndpointURL string

	// UsePathStyle forces path-style bucket addressing (http://host/bucket/key
	// instead of http://bucket.host/key). Required for MinIO and self-hosted
	// Ceph. Set to false for AWS S3 and Cloudflare R2.
	UsePathStyle bool

	// DisableSSL is no longer needed — set EndpointURL to an http:// URL for
	// local MinIO testing. This field is retained for backwards compatibility
	// but has no effect; the scheme in EndpointURL controls TLS.
	//
	// Deprecated: set EndpointURL to "http://..." instead.
	DisableSSL bool
}

// s3Backend is the S3-compatible implementation of Backend.
type s3Backend struct {
	client    *s3.Client
	bucket    string
	keyPrefix string // path component from the s3:// URL, without leading slash
}

// NewS3 constructs an S3-compatible Backend from cfg.
//
// Credentials come from the AWS SDK's default credential chain:
// environment variables (AWS_ACCESS_KEY_ID / AWS_SECRET_ACCESS_KEY),
// shared credentials file (~/.aws/credentials), web-identity token (IRSA),
// and EC2/ECS instance metadata (IMDS). For MinIO, set
// AWS_ACCESS_KEY_ID and AWS_SECRET_ACCESS_KEY in the environment or use
// github.com/aws/aws-sdk-go-v2/credentials.NewStaticCredentialsProvider and
// pass it via config.WithCredentialsProvider.
//
// The returned Backend is safe for concurrent use.
func NewS3(cfg S3Config) (Backend, error) {
	bucket, keyPrefix, err := parseS3URL(cfg.URL)
	if err != nil {
		return nil, fmt.Errorf("objectstore/s3: invalid URL %q: %w", cfg.URL, err)
	}

	// Build AWS config load options.
	var loadOpts []func(*config.LoadOptions) error
	if cfg.Region != "" {
		loadOpts = append(loadOpts, config.WithRegion(cfg.Region))
	}
	if cfg.EndpointURL != "" {
		loadOpts = append(loadOpts, config.WithBaseEndpoint(cfg.EndpointURL))
	}

	awsCfg, err := config.LoadDefaultConfig(context.Background(), loadOpts...)
	if err != nil {
		return nil, fmt.Errorf("objectstore/s3: load AWS config: %w", err)
	}

	// Build S3 client options.
	var s3Opts []func(*s3.Options)
	if cfg.UsePathStyle {
		s3Opts = append(s3Opts, func(o *s3.Options) {
			o.UsePathStyle = true
		})
	}

	client := s3.NewFromConfig(awsCfg, s3Opts...)
	return &s3Backend{
		client:    client,
		bucket:    bucket,
		keyPrefix: keyPrefix,
	}, nil
}

// parseS3URL parses a URL of the form s3://bucket/optional/prefix and
// returns (bucket, prefix, nil). The prefix has no leading slash.
func parseS3URL(rawURL string) (bucket, prefix string, err error) {
	u, err := url.Parse(rawURL)
	if err != nil {
		return "", "", err
	}
	if u.Scheme != "s3" {
		return "", "", fmt.Errorf("scheme must be s3, got %q", u.Scheme)
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
func (b *s3Backend) fullKey(key string) string {
	if b.keyPrefix == "" {
		return key
	}
	return b.keyPrefix + "/" + key
}

// logicalKey strips the key prefix from a full S3 object key, returning
// the logical key the caller should see.
func (b *s3Backend) logicalKey(fullKey string) string {
	if b.keyPrefix == "" {
		return fullKey
	}
	prefix := b.keyPrefix + "/"
	return strings.TrimPrefix(fullKey, prefix)
}

// stripEtag removes the surrounding double-quotes that AWS wraps ETags in.
// S3 returns ETags as `"abc123"` — we store and compare without quotes so
// callers can round-trip without worrying about the quoting convention.
func stripEtag(etag *string) string {
	if etag == nil {
		return ""
	}
	return strings.Trim(*etag, `"`)
}

// Put implements Backend.Put.
func (b *s3Backend) Put(ctx context.Context, key string, data []byte, fencingToken int64, ifMatch string) (string, error) {
	input := &s3.PutObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.fullKey(key)),
		Body:   bytes.NewReader(data),
		Metadata: map[string]string{
			metaKeyFencingToken: strconv.FormatInt(fencingToken, 10),
		},
	}
	if ifMatch != "" {
		input.IfMatch = aws.String(ifMatch)
	}

	out, err := b.client.PutObject(ctx, input)
	if err != nil {
		return "", mapS3Error(err)
	}
	return stripEtag(out.ETag), nil
}

// PutIdempotent implements Backend.PutIdempotent.
//
// Strategy: attempt a create-only write using IfNoneMatch: "*". If the object
// already exists (412 PreconditionFailed), fetch its current content and
// compare byte-for-byte. Return nil if contents match; ErrAlreadyExists if
// they differ.
func (b *s3Backend) PutIdempotent(ctx context.Context, key string, data []byte, fencingToken int64) error {
	input := &s3.PutObjectInput{
		Bucket:      aws.String(b.bucket),
		Key:         aws.String(b.fullKey(key)),
		Body:        bytes.NewReader(data),
		IfNoneMatch: aws.String("*"),
		Metadata: map[string]string{
			metaKeyFencingToken: strconv.FormatInt(fencingToken, 10),
		},
	}

	_, err := b.client.PutObject(ctx, input)
	if err == nil {
		return nil // successfully created
	}

	// Check whether the error is a precondition failure (object already exists).
	if !isPreconditionFailed(err) {
		return mapS3Error(err)
	}

	// Object exists. Fetch its content and compare.
	existing, _, _, getErr := b.Get(ctx, key)
	if getErr != nil {
		return fmt.Errorf("objectstore/s3: PutIdempotent: read existing object: %w", getErr)
	}

	if bytes.Equal(existing, data) {
		return nil // same content — idempotent success
	}
	return ErrAlreadyExists
}

// Get implements Backend.Get.
func (b *s3Backend) Get(ctx context.Context, key string) ([]byte, string, int64, error) {
	out, err := b.client.GetObject(ctx, &s3.GetObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.fullKey(key)),
	})
	if err != nil {
		return nil, "", 0, mapS3Error(err)
	}
	defer out.Body.Close()

	data, err := io.ReadAll(out.Body)
	if err != nil {
		return nil, "", 0, fmt.Errorf("objectstore/s3: read body: %w", err)
	}

	etag := stripEtag(out.ETag)
	var fencingToken int64
	if tokenStr, ok := out.Metadata[metaKeyFencingToken]; ok && tokenStr != "" {
		fencingToken, err = strconv.ParseInt(tokenStr, 10, 64)
		if err != nil {
			// Malformed metadata is not fatal — return 0 (the "no fencing"
			// sentinel) and let the caller decide how to handle it.
			fencingToken = 0
		}
	}

	return data, etag, fencingToken, nil
}

// Delete implements Backend.Delete.
//
// S3's DeleteObject is idempotent by specification — deleting a non-existent
// key returns success.
func (b *s3Backend) Delete(ctx context.Context, key string) error {
	_, err := b.client.DeleteObject(ctx, &s3.DeleteObjectInput{
		Bucket: aws.String(b.bucket),
		Key:    aws.String(b.fullKey(key)),
	})
	if err != nil {
		return mapS3Error(err)
	}
	return nil
}

// List implements Backend.List.
//
// Uses the ListObjectsV2 paginator so arbitrarily large result sets are
// handled correctly. Keys are delivered in lexicographic (UTF-8 byte) order,
// which is what S3 guarantees. The key prefix (if configured) is stripped
// before the key is passed to fn.
func (b *s3Backend) List(ctx context.Context, prefix string, fn func(key string) error) error {
	fullPrefix := b.fullKey(prefix)
	paginator := s3.NewListObjectsV2Paginator(b.client, &s3.ListObjectsV2Input{
		Bucket: aws.String(b.bucket),
		Prefix: aws.String(fullPrefix),
	})

	for paginator.HasMorePages() {
		page, err := paginator.NextPage(ctx)
		if err != nil {
			return fmt.Errorf("objectstore/s3: list page: %w", mapS3Error(err))
		}
		for _, obj := range page.Contents {
			if obj.Key == nil {
				continue
			}
			logKey := b.logicalKey(*obj.Key)
			if err := fn(logKey); err != nil {
				return err // caller requested early exit
			}
		}
	}
	return nil
}

// mapS3Error converts S3 / smithy errors to the objectstore sentinel errors.
// Unknown errors are returned unchanged.
func mapS3Error(err error) error {
	if err == nil {
		return nil
	}
	// Check for the typed NoSuchKey error first.
	var noSuchKey *s3types.NoSuchKey
	if errors.As(err, &noSuchKey) {
		return ErrNotFound
	}
	var notFound *s3types.NotFound
	if errors.As(err, &notFound) {
		return ErrNotFound
	}

	// Fall back to the smithy APIError code string for errors that don't have
	// typed representations (e.g. PreconditionFailed from some S3-compat
	// providers).
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		switch apiErr.ErrorCode() {
		case "NoSuchKey", "NotFound", "404":
			return ErrNotFound
		case "PreconditionFailed", "ConditionNotMet":
			return ErrPrecondition
		}
	}
	return err
}

// isPreconditionFailed reports whether err is a 412 PreconditionFailed from S3.
func isPreconditionFailed(err error) bool {
	if err == nil {
		return false
	}
	var apiErr smithy.APIError
	if errors.As(err, &apiErr) {
		code := apiErr.ErrorCode()
		return code == "PreconditionFailed" || code == "ConditionNotMet"
	}
	return false
}
