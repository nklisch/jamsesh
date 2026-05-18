// Package minio provides a Testcontainers-Go fixture for MinIO (S3-compatible
// object storage) used by the e2e clustered-mode test suite.
//
// Each Start call spins up a fresh MinIO container and pre-creates a random
// bucket. The container is isolated per-test — no shared singleton — so tests
// get independent object storage without cross-test interference.
//
// Usage:
//
//	m := minio.Start(ctx, t, minio.Options{})
//	// m.Endpoint is the S3 endpoint from the test process, e.g. "http://127.0.0.1:32781"
//	// m.BucketName is pre-created and ready for use
package minio

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"os/exec"
	"testing"
	"time"

	miniogo "github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/wait"

	"jamsesh/tests/e2e/fixtures/containerlog"
)

const (
	minioImage     = "minio/minio:RELEASE.2024-12-18T13-15-44Z"
	minioAPIPort   = "9000/tcp"
	minioAccessKey = "minioadmin"
	minioSecretKey = "minioadmin"
)

// Options configures the MinIO container.
type Options struct {
	// ExtraEnv passes additional MINIO_* env vars to the container.
	ExtraEnv map[string]string
}

// MinIO holds connection info for a running MinIO container with a pre-created
// per-test bucket.
type MinIO struct {
	// Endpoint is the host-side S3 endpoint, e.g. "http://127.0.0.1:32781".
	// Use this from the test process.
	Endpoint string

	// ContainerEndpoint is the bridge-IP endpoint, e.g. "http://172.18.0.5:9000".
	// Use this when configuring another container (e.g. portal) to reach MinIO.
	ContainerEndpoint string

	AccessKey  string // "minioadmin" by default
	SecretKey  string // "minioadmin" by default
	BucketName string // random per-test, pre-created

	container testcontainers.Container
}

// Start spins up a fresh MinIO container, waits for it to be healthy, creates
// a random bucket, and registers t.Cleanup to terminate the container.
// The test is skipped cleanly if Docker is unavailable.
func Start(ctx context.Context, t *testing.T, opts Options) *MinIO {
	t.Helper()
	requireDocker(t)

	env := map[string]string{
		"MINIO_ROOT_USER":     minioAccessKey,
		"MINIO_ROOT_PASSWORD": minioSecretKey,
	}
	for k, v := range opts.ExtraEnv {
		env[k] = v
	}

	req := testcontainers.GenericContainerRequest{
		ContainerRequest: testcontainers.ContainerRequest{
			Image:        minioImage,
			ExposedPorts: []string{minioAPIPort},
			Env:          env,
			Cmd:          []string{"server", "/data"},
			WaitingFor: wait.ForHTTP("/minio/health/live").
				WithPort(minioAPIPort).
				WithStatusCodeMatcher(func(code int) bool { return code == 200 }).
				WithStartupTimeout(30 * time.Second),
		},
		Started: true,
	}

	c, err := testcontainers.GenericContainer(ctx, req)
	if err != nil {
		t.Fatalf("minio: start container: %v", err)
	}

	t.Cleanup(func() {
		containerlog.DumpAndTerminate(ctx, t, c, "minio")
	})

	host, err := c.Host(ctx)
	if err != nil {
		t.Fatalf("minio: get host: %v", err)
	}
	mappedPort, err := c.MappedPort(ctx, minioAPIPort)
	if err != nil {
		t.Fatalf("minio: get port: %v", err)
	}

	containerIP, err := c.ContainerIP(ctx)
	if err != nil {
		t.Fatalf("minio: get container IP: %v", err)
	}

	endpoint := fmt.Sprintf("http://%s:%d", host, mappedPort.Num())
	containerEndpoint := fmt.Sprintf("http://%s:9000", containerIP)

	// Pre-create a random bucket for test isolation.
	// S3/MinIO bucket names allow lowercase letters, numbers, and hyphens only.
	bucketName := "bucket-" + randHex(4)

	endpointHost := fmt.Sprintf("%s:%d", host, mappedPort.Num())
	mc, err := miniogo.New(endpointHost, &miniogo.Options{
		Creds:  credentials.NewStaticV4(minioAccessKey, minioSecretKey, ""),
		Secure: false,
	})
	if err != nil {
		t.Fatalf("minio: create client: %v", err)
	}

	if err := mc.MakeBucket(ctx, bucketName, miniogo.MakeBucketOptions{}); err != nil {
		t.Fatalf("minio: create bucket %q: %v", bucketName, err)
	}

	return &MinIO{
		Endpoint:          endpoint,
		ContainerEndpoint: containerEndpoint,
		AccessKey:         minioAccessKey,
		SecretKey:         minioSecretKey,
		BucketName:        bucketName,
		container:         c,
	}
}

// randHex returns n hex-encoded random bytes (2*n characters).
func randHex(n int) string {
	b := make([]byte, n)
	if _, err := rand.Read(b); err != nil {
		panic(fmt.Sprintf("minio: rand.Read: %v", err))
	}
	return hex.EncodeToString(b)
}

// requireDocker skips t if the Docker daemon is not reachable.
func requireDocker(t *testing.T) {
	t.Helper()
	if err := exec.Command("docker", "info").Run(); err != nil {
		t.Skip("docker not available")
	}
}
