// Invariant: the portal boots cleanly when JAMSESH_DB_DSN_FILE points at a
// mounted file containing a valid Postgres DSN. The _FILE variant takes
// precedence over the plain JAMSESH_DB_DSN env var — this is the secret-file
// indirection used in production Kubernetes deployments. A silent fallback to
// an empty or in-memory DSN would be a production footgun; /healthz 200
// (portal.Start's wait strategy) proves the portal is fully operational.
package golden_test

import (
	"context"
	"net/http"
	"os"
	"path/filepath"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

func TestFileSecretHappyPath(t *testing.T) {
	ctx := context.Background()

	pg := postgres.Start(ctx, t, postgres.Options{})

	// Write the DSN to a host-side temp file. The file will be mounted into
	// the portal container at /run/secrets/db_dsn — mirroring how Docker
	// Swarm / Kubernetes inject secrets at runtime.
	dsnFile := filepath.Join(t.TempDir(), "db_dsn")
	require.NoError(t, os.WriteFile(dsnFile, []byte(pg.ContainerDSN), 0o600),
		"write DSN secret file")

	// buildEnv sets JAMSESH_DB_DSN from opts.DBDSN (defaulting to ":memory:"
	// when empty). ExtraEnv runs last and overwrites the key, so setting
	// JAMSESH_DB_DSN="" via ExtraEnv effectively clears the plain DSN.
	// readEnvOrFile's precedence is: _FILE wins when set, regardless of the
	// plain env var — but clearing the plain DSN makes it unambiguous that
	// _FILE is the sole source and prevents a stale value from masking a
	// broken mount.
	p := portal.Start(ctx, t, portal.Options{
		DBDriver:  "postgres",
		// DBDSN intentionally empty — ExtraEnv clears the key that buildEnv
		// would otherwise fill with ":memory:".
		EmailFrom: "noreply@example.com",
		ExtraEnv: map[string]string{
			"JAMSESH_DB_DSN":      "",                     // clear the buildEnv default
			"JAMSESH_DB_DSN_FILE": "/run/secrets/db_dsn", // _FILE indirection
		},
		ContainerFiles: []testcontainers.ContainerFile{
			{
				HostFilePath:      dsnFile,
				ContainerFilePath: "/run/secrets/db_dsn",
				FileMode:          0o644, // readable by the container's nobody user
			},
		},
	})

	// portal.Start already waited for /healthz 200 — this extra call is an
	// explicit assertion that the portal is still serving after boot, and
	// produces a clearer failure message if the health check races with a
	// late startup crash.
	resp, err := http.Get(p.URL + "/healthz")
	require.NoError(t, err, "GET /healthz")
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode,
		"portal must be healthy when DSN is supplied via _FILE secret mount")
}
