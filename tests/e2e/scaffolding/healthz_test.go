package scaffolding_test

import (
	"context"
	"io"
	"net/http"
	"path/filepath"
	"testing"

	"jamsesh/tests/e2e/fixtures/mailhog"
	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
	"jamsesh/tests/e2e/fixtures/toxiproxy"
	"jamsesh/tests/e2e/fixtures/wiremock"
)

// TestPortalHealthz is the e2e proof-of-life: with the full Testcontainers
// stack (Postgres + MailHog + WireMock + Toxiproxy + portal), GET /healthz
// returns 200. If this passes, the rest of the e2e program has a working
// foundation.
//
// The test skips cleanly on machines without Docker (via requireDocker inside
// each fixture) or without the portal image (via requirePortalImage inside the
// portal fixture). No build tag is needed — run `go test ./scaffolding/` to
// invoke it explicitly.
//
// Prerequisites:
//   - Docker running
//   - Portal image built: `make test-portal-image`
func TestPortalHealthz(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})

	mh := mailhog.Start(ctx, t)

	// Mount GitHub OAuth stubs. Path is relative to this test file's package
	// directory, which is tests/e2e/scaffolding/. The mappings file lives at
	// tests/e2e/fixtures/wiremock/mappings/github.json.
	githubMappings, err := filepath.Abs("../fixtures/wiremock/mappings/github.json")
	if err != nil {
		t.Fatalf("resolve wiremock mappings path: %v", err)
	}
	wm := wiremock.Start(ctx, t, wiremock.Mappings{
		"github": githubMappings,
	})

	// Toxiproxy is included to prove it starts correctly as part of the
	// infrastructure baseline. The smoke spec does not inject toxics.
	_ = toxiproxy.Start(ctx, t)

	p := portal.Start(ctx, t, portal.Options{
		DBDriver: "postgres",
		// ContainerDSN uses the Postgres container's Docker bridge IP (port
		// 5432). The portal runs inside Docker, so the host-mapped pg.DSN
		// is not reachable from inside the portal container.
		DBDSN:     pg.ContainerDSN,
		EmailFrom: "noreply@example.com",
		// ContainerSMTPHost / ContainerURL use Docker bridge IPs so the portal
		// container can reach MailHog and WireMock across the bridge network.
		SMTPHost:     mh.ContainerSMTPHost,
		SMTPPort:     mh.ContainerSMTPPort,
		OAuthBaseURL: wm.ContainerURL,
	})

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
