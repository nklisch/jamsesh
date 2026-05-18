package minio_test

import (
	"context"
	"testing"

	"jamsesh/tests/e2e/fixtures/minio"
)

func TestMinIOStart(t *testing.T) {
	ctx := context.Background()
	m := minio.Start(ctx, t, minio.Options{})

	// PUT a small payload.
	key := "smoke/hello.txt"
	payload := []byte("hello, minio")
	if err := m.PutObject(ctx, key, payload); err != nil {
		t.Fatalf("PutObject: %v", err)
	}

	// GET it back.
	got, err := m.GetObject(ctx, key)
	if err != nil {
		t.Fatalf("GetObject: %v", err)
	}
	if string(got) != string(payload) {
		t.Errorf("GetObject: got %q, want %q", got, payload)
	}

	// ListObjects.
	keys, err := m.ListObjects(ctx, "smoke/")
	if err != nil {
		t.Fatalf("ListObjects: %v", err)
	}
	if len(keys) != 1 || keys[0] != key {
		t.Errorf("ListObjects: got %v, want [%q]", keys, key)
	}
}
