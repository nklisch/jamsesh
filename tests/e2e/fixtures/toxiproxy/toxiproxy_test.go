package toxiproxy_test

import (
	"context"
	"testing"

	"jamsesh/tests/e2e/fixtures/toxiproxy"
)

// TestStartToxiproxy verifies that Start brings up a Toxiproxy container and
// that the admin API is reachable.
func TestStartToxiproxy(t *testing.T) {
	ctx := context.Background()
	tp := toxiproxy.Start(ctx, t)

	if tp.AdminURL == "" {
		t.Fatal("expected non-empty AdminURL")
	}

	if err := tp.CheckReachable(); err != nil {
		t.Fatalf("toxiproxy admin API not reachable: %v", err)
	}
}
