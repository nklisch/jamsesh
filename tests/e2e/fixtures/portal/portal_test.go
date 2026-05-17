package portal_test

import (
	"context"
	"io"
	"net/http"
	"testing"

	"jamsesh/tests/e2e/fixtures/portal"
)

// TestStartPortalSQLite verifies that Start brings up the portal using SQLite
// in-memory and that /healthz returns 200. Skips if the image is absent or
// Docker is unavailable.
//
// This test exercises only the portal fixture in isolation. For the full-stack
// smoke spec (with Postgres + MailHog + WireMock + Toxiproxy) see
// tests/e2e/scaffolding/healthz_test.go.
func TestStartPortalSQLite(t *testing.T) {
	ctx := context.Background()
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "sqlite",
		DBDSN:     ":memory:",
		EmailFrom: "noreply@example.com",
	})

	if p.URL == "" {
		t.Fatal("expected non-empty URL")
	}

	resp, err := http.Get(p.URL + "/healthz") //nolint:noctx
	if err != nil {
		t.Fatalf("GET /healthz: %v", err)
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status: got %d, want 200; body=%q", resp.StatusCode, body)
	}
}
