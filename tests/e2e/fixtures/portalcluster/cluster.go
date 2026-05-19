// Package portalcluster provides a Testcontainers-Go fixture for a multi-pod
// clustered-mode jamsesh portal deployment. Used by the cnd-coverage e2e tests
// for lease-fencing, object-storage-sync, hydration-handoff, and routing-layer
// scenarios.
//
// Usage:
//
//	pg := postgres.Start(ctx, t, postgres.Options{})
//	mn := minio.Start(ctx, t, minio.Options{})
//
//	c := portalcluster.Start(ctx, t, portalcluster.Options{
//	    Pods:        2,
//	    Postgres:    pg,
//	    ObjectStore: mn,
//	    Router:      true,
//	})
//	// c.RouterURL is the front door when Router: true
//	// c.Pods[i].URL addresses each pod directly
package portalcluster

import (
	"context"
	"fmt"
	"sync"
	"testing"

	"golang.org/x/sync/errgroup"

	"jamsesh/tests/e2e/fixtures/minio"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/router"
)

// Options configures the clustered portal fixture.
type Options struct {
	// Pods is the number of portal containers to start. Default 2.
	Pods int

	// Postgres is REQUIRED — the cluster shares one Postgres DB across pods.
	// Obtain from postgres.Start.
	Postgres *postgres.Postgres

	// ObjectStore is REQUIRED for clustered-mode boot. The cluster fixture
	// sets JAMSESH_DEPLOY_MODE=clustered and JAMSESH_OBJECT_STORAGE_URL.
	// Obtain from minio.Start.
	ObjectStore *minio.MinIO

	// Router, if true, starts a jamsesh-router container fronting the pods.
	// If false, consumers address pods directly via Cluster.Pods[i].URL.
	Router bool

	// PortalExtraEnv passes extra JAMSESH_* vars to each portal container.
	// Keys must be full env-var names (e.g. "JAMSESH_LEASE_HEARTBEAT_INTERVAL_S").
	// These override the clustered-mode defaults set by the fixture.
	PortalExtraEnv map[string]string
}

// Cluster holds connection info for a running clustered portal deployment.
type Cluster struct {
	// RouterURL is the front door when Options.Router was true; empty otherwise.
	RouterURL string

	// Pods is the slice of started portal containers, one per requested pod.
	// Pods[i].URL is the host-side base URL for pod i.
	Pods []*portal.Portal

	// internal references for lifecycle helpers
	postgres    *postgres.Postgres
	objectStore *minio.MinIO
	rtr         *router.Router // nil when Options.Router was false
}

// Start spins up a clustered portal deployment: N portal containers configured
// for clustered mode against shared Postgres + MinIO, and optionally a
// jamsesh-router fronting them.
//
// All pods are started in parallel via errgroup.Group — a failure in any pod
// aborts the group and t.Fatal is called, preventing partial clusters from
// wasting developer time.
//
// If Docker or the jamsesh/portal:e2e image is unavailable, the test is skipped
// (propagated from the underlying portal.Start call).
func Start(ctx context.Context, t *testing.T, opts Options) *Cluster {
	t.Helper()

	if opts.Postgres == nil {
		t.Fatal("portalcluster.Start: opts.Postgres is nil — pass a postgres.Start result")
	}
	if opts.ObjectStore == nil {
		t.Fatal("portalcluster.Start: opts.ObjectStore is nil — pass a minio.Start result")
	}
	if opts.Pods == 0 {
		opts.Pods = 2
	}

	// Compose the shared clustered-mode env vars that every pod receives.
	// Env-var names verified against internal/portal/config/config.go.
	sharedEnv := map[string]string{
		"JAMSESH_DEPLOY_MODE": "clustered",

		// Object storage — point each pod at the MinIO bucket created by the fixture.
		// The portal requires an s3:// URL (not s3-compatible://) when
		// JAMSESH_OBJECT_STORAGE_ENDPOINT_URL is set to override the endpoint.
		"JAMSESH_OBJECT_STORAGE_URL":        "s3://" + opts.ObjectStore.BucketName + "/",
		"JAMSESH_OBJECT_STORAGE_ENDPOINT_URL": opts.ObjectStore.ContainerEndpoint,
		"JAMSESH_OBJECT_STORAGE_PATH_STYLE":  "true",
		"JAMSESH_OBJECT_STORAGE_REGION":      "us-east-1",

		// S3 credentials for MinIO. These are the standard AWS SDK env vars;
		// the portal's object-storage layer passes them to the AWS SDK which
		// reads them automatically — there are no JAMSESH_OBJECT_STORAGE_ACCESS_KEY*
		// vars in config.go. MinIO defaults to minioadmin/minioadmin.
		"AWS_ACCESS_KEY_ID":     opts.ObjectStore.AccessKey,
		"AWS_SECRET_ACCESS_KEY": opts.ObjectStore.SecretKey,
	}

	// Caller-supplied overrides on top of clustered-mode defaults.
	for k, v := range opts.PortalExtraEnv {
		sharedEnv[k] = v
	}

	// Start all pods in parallel. errgroup propagates the first error and
	// cancels the rest; t.Fatal is called if any pod fails to start.
	pods := make([]*portal.Portal, opts.Pods)
	var mu sync.Mutex // protects pods slice during parallel writes

	eg, egCtx := errgroup.WithContext(ctx)
	for i := 0; i < opts.Pods; i++ {
		i := i // capture loop variable
		eg.Go(func() error {
			p := portal.Start(egCtx, t, portal.Options{
				DBDriver:  "postgres",
				DBDSN:     opts.Postgres.ContainerDSN,
				EmailFrom: "noreply@example.com",
				ExtraEnv:  sharedEnv,
			})
			mu.Lock()
			pods[i] = p
			mu.Unlock()
			return nil
		})
	}

	if err := eg.Wait(); err != nil {
		// portal.Start calls t.Fatal on failure, so we should not reach here
		// unless the errgroup itself has an error (e.g. context cancelled).
		t.Fatalf("portalcluster.Start: pod startup failed: %v", err)
	}

	// Verify all pods started (nil entries indicate a t.Fatal or skip propagated
	// up — in practice t.Fatal would have aborted the test already).
	for i, p := range pods {
		if p == nil {
			t.Fatalf("portalcluster.Start: pod %d is nil after startup — check portal.Start logs", i)
		}
	}

	c := &Cluster{
		Pods:        pods,
		postgres:    opts.Postgres,
		objectStore: opts.ObjectStore,
	}

	if opts.Router {
		// Collect each pod's container IP for the router's backend list.
		// Pods communicate with the router on the Docker bridge network at
		// port 8443 (the portal's internal bind port).
		//
		// Also collect each pod's host-side URL for the pre-start readyz poll.
		// The test process cannot reach container-internal IPs directly, so
		// we use the host-mapped port URL that Testcontainers exposes.
		backends := make([]string, len(pods))
		readyzURLs := make([]string, len(pods))
		for i, p := range pods {
			ip, err := p.ContainerIP(ctx)
			if err != nil {
				t.Fatalf("portalcluster.Start: get container IP for pod %d: %v", i, err)
			}
			backends[i] = fmt.Sprintf("%s:8443", ip)
			readyzURLs[i] = p.URL // host-side URL, e.g. "http://127.0.0.1:PORT"
		}

		rtr := router.Start(ctx, t, router.Options{
			Backends:          backends,
			BackendReadyzURLs: readyzURLs,
		})
		c.rtr = rtr
		c.RouterURL = rtr.URL
	}

	return c
}
