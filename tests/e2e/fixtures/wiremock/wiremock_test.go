package wiremock_test

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"jamsesh/tests/e2e/fixtures/wiremock"
)

// TestStartWireMock verifies that Start brings up a WireMock container, that
// the admin API is reachable, and that the mounted GitHub OAuth stubs return
// the expected responses.
func TestStartWireMock(t *testing.T) {
	ctx := context.Background()

	// Resolve the mappings path relative to the module root so the test works
	// when invoked from any directory under tests/e2e/.
	githubMappings, err := filepath.Abs("mappings/github.json")
	if err != nil {
		t.Fatalf("resolve path: %v", err)
	}

	wm := wiremock.Start(ctx, t, wiremock.Mappings{
		"github": githubMappings,
	})

	if wm.URL == "" {
		t.Fatal("expected non-empty URL")
	}

	if err := wm.CheckReachable(); err != nil {
		t.Fatalf("wiremock admin API not reachable: %v", err)
	}

	// Verify the GitHub user stub returns the expected fixture data.
	resp, err := http.Get(wm.URL + "/user") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /user: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /user: status %d, body: %s", resp.StatusCode, body)
	}
}
