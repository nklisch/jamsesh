package portalcluster_test

import (
	"context"
	"net/http"
	"testing"

	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portalcluster"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// TestClusterStart verifies the core fixture invariant: a 3-pod cluster in
// direct-pod mode (Router: false) boots successfully and each pod answers
// /healthz with 200 OK.
//
// Prerequisites:
//   - Docker daemon running
//   - jamsesh/portal:e2e image built (`make test-portal-image`)
//   - MinIO image available (pulled automatically by testcontainers)
//
// The test is skipped cleanly when Docker is unavailable or the portal image
// has not been built. Allow up to 5 minutes — 3 portal containers starting in
// parallel plus healthz polling can take ~90 seconds on a cold image cache.
func TestClusterStart(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})
	mn := minio.Start(ctx, t, minio.Options{})

	c := portalcluster.Start(ctx, t, portalcluster.Options{
		Pods:        3,
		Postgres:    pg,
		ObjectStore: mn,
		Router:      false, // direct-pod access for the self-test
	})

	if len(c.Pods) != 3 {
		t.Fatalf("cluster must have 3 pods, got %d", len(c.Pods))
	}
	if c.RouterURL != "" {
		t.Fatalf("RouterURL must be empty when Router: false, got %q", c.RouterURL)
	}

	// Each pod must answer /healthz 200. This is the real invariant —
	// running status alone is not sufficient (running != healthy).
	for i, p := range c.Pods {
		resp, err := http.Get(p.URL + "/healthz") //nolint:noctx // direct health check; no per-request context needed
		if err != nil {
			t.Errorf("pod %d /healthz: %v", i, err)
			continue
		}
		resp.Body.Close()
		if resp.StatusCode != http.StatusOK {
			t.Errorf("pod %d /healthz: status %d (want 200)", i, resp.StatusCode)
		}
	}
}
