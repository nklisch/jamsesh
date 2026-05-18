package minio

import (
	"bytes"
	"context"
	"fmt"
	"io"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

// client returns a minio-go client pointed at m.Endpoint.
// The minio-go SDK takes a host:port form, not a full URL, so we strip the
// scheme prefix.
func (m *MinIO) client() (*miniogo.Client, error) {
	// Strip leading "http://" or "https://" from the endpoint.
	host := m.Endpoint
	for _, prefix := range []string{"https://", "http://"} {
		if len(host) > len(prefix) && host[:len(prefix)] == prefix {
			host = host[len(prefix):]
			break
		}
	}
	return miniogo.New(host, &miniogo.Options{
		Creds:  credentials.NewStaticV4(m.AccessKey, m.SecretKey, ""),
		Secure: false,
	})
}

// ListObjects returns the keys of objects in the bucket matching the given
// prefix. An empty prefix lists all objects.
func (m *MinIO) ListObjects(ctx context.Context, prefix string) ([]string, error) {
	mc, err := m.client()
	if err != nil {
		return nil, fmt.Errorf("minio: list objects: create client: %w", err)
	}

	var keys []string
	for obj := range mc.ListObjects(ctx, m.BucketName, miniogo.ListObjectsOptions{
		Prefix:    prefix,
		Recursive: true,
	}) {
		if obj.Err != nil {
			return nil, fmt.Errorf("minio: list objects: %w", obj.Err)
		}
		keys = append(keys, obj.Key)
	}
	return keys, nil
}

// GetObject fetches an object's bytes from the bucket.
func (m *MinIO) GetObject(ctx context.Context, key string) ([]byte, error) {
	mc, err := m.client()
	if err != nil {
		return nil, fmt.Errorf("minio: get object %q: create client: %w", key, err)
	}

	obj, err := mc.GetObject(ctx, m.BucketName, key, miniogo.GetObjectOptions{})
	if err != nil {
		return nil, fmt.Errorf("minio: get object %q: %w", key, err)
	}
	defer obj.Close()

	data, err := io.ReadAll(obj)
	if err != nil {
		return nil, fmt.Errorf("minio: get object %q: read: %w", key, err)
	}
	return data, nil
}

// PutObject writes raw bytes to a key in the bucket.
func (m *MinIO) PutObject(ctx context.Context, key string, data []byte) error {
	mc, err := m.client()
	if err != nil {
		return fmt.Errorf("minio: put object %q: create client: %w", key, err)
	}

	_, err = mc.PutObject(ctx, m.BucketName, key, bytes.NewReader(data), int64(len(data)), miniogo.PutObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: put object %q: %w", key, err)
	}
	return nil
}

// DeleteObject removes a key from the bucket.
func (m *MinIO) DeleteObject(ctx context.Context, key string) error {
	mc, err := m.client()
	if err != nil {
		return fmt.Errorf("minio: delete object %q: create client: %w", key, err)
	}

	err = mc.RemoveObject(ctx, m.BucketName, key, miniogo.RemoveObjectOptions{})
	if err != nil {
		return fmt.Errorf("minio: delete object %q: %w", key, err)
	}
	return nil
}
