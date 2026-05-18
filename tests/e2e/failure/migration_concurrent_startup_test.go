// Invariant: when N portal replicas start simultaneously against a fresh
// Postgres database (as in a rolling deploy), exactly one replica applies
// the schema migrations. All N replicas must come up healthy.
//
// This test is the e2e existence proof that the pg_advisory_lock(8675309)
// mechanism in internal/db/migrate.go serialises concurrent DDL — if the
// lock were absent, two pods could interleave CREATE TABLE statements and
// corrupt the schema.
//
// Assertion strategy:
//  1. Start 3 portal containers in parallel against the same fresh DB.
//  2. Wait for all 3 to pass /healthz (portal.Start blocks until healthy).
//  3. Inspect each container's logs. The portal binary emits
//     `"db migrations applied"` via slog (internal/db/migrate.go) only when
//     actual DDL ran — an idempotent run against an already-current DB is
//     silent. Exactly one container must have this phrase.
//  4. Post-condition: query goose_db_version and assert MAX(version_id) > 0,
//     confirming the schema is at a real version.
//
// Advisory-lock failure mode: if 2 or more containers report applying
// migrations, that is a Critical bug — split-brain DDL during a rolling
// deploy. The assertion is exact (== 1), not >= 1.
package failure_test

import (
	"context"
	"database/sql"
	"net/http"
	"strings"
	"sync"
	"testing"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// migrationLogPhrase is the exact substring we grep for in container logs.
// It is emitted by internal/db/migrate.go via slog.InfoContext when actual
// migrations ran (len(results) > 0). The portal uses JSON slog format so the
// phrase appears as: {"msg":"db migrations applied",...}
// We match on the msg value only — JSON field ordering is implementation detail.
const migrationLogPhrase = "db migrations applied"

func TestMigrationConcurrentStartup(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	ctx := context.Background()

	// Start a fresh per-test Postgres DB with no schema yet. The portal
	// replicas will be the first callers — exactly one must run DDL.
	pg := postgres.Start(ctx, t, postgres.Options{})

	const N = 3

	portals := make([]*portal.Portal, N)
	var mu sync.Mutex

	var g errgroup.Group
	for i := 0; i < N; i++ {
		i := i
		g.Go(func() error {
			p := portal.Start(ctx, t, portal.Options{
				DBDriver:  "postgres",
				DBDSN:     pg.ContainerDSN,
				EmailFrom: "noreply@example.com",
				// No SMTP/OAuth wiring: this test exercises the startup DDL path
				// only. /healthz does not depend on mail delivery or GitHub OAuth.
			})
			mu.Lock()
			portals[i] = p
			mu.Unlock()
			return nil
		})
	}

	// portal.Start fatally fails on error inside the goroutine (via t.Fatalf),
	// so g.Wait() always returns nil if control returns to this point. The
	// require is belt-and-suspenders in case future Start refactors return errors.
	require.NoError(t, g.Wait(), "all portals must start healthy")

	// Re-assert /healthz for each portal. portal.Start's wait strategy already
	// guarantees 200, but explicit assertion here documents the invariant and
	// catches a regression if the wait strategy is ever weakened.
	for i, p := range portals {
		resp, err := http.Get(p.URL + "/healthz") //nolint:noctx
		require.NoErrorf(t, err, "portal[%d] /healthz", i)
		require.Equalf(t, http.StatusOK, resp.StatusCode, "portal[%d] /healthz", i)
		resp.Body.Close()
	}

	// Inspect each container's log stream. Exactly one should contain the
	// migration-applied phrase. Goose's advisory-lock serialisation ensures the
	// first holder runs MigrateUp with a non-empty results slice; the remaining
	// holders call MigrateUp after the schema exists, get an empty results slice,
	// and emit nothing.
	migratorCount := 0
	var migratorIDs []int
	for i, p := range portals {
		logs, err := p.Logs(ctx)
		if err != nil {
			t.Logf("portal[%d]: read container logs: %v", i, err)
			// Continue — a read failure is not a migration-count bug; we'll
			// catch it in the require.Equal assertion below.
		}
		if strings.Contains(logs, migrationLogPhrase) {
			migratorCount++
			migratorIDs = append(migratorIDs, i)
		}
	}

	// CRITICAL invariant: exactly one pod must have applied migrations.
	// If 0: the migration log phrase is missing — the slog call regressed.
	// If 2+: the advisory lock is not serialising DDL — this is a production hazard.
	// Neither case is acceptable. Do NOT loosen this to >= 1.
	require.Equalf(t, 1, migratorCount,
		"exactly one portal must apply migrations; got %d (portals that matched: %v). "+
			"If 2+: advisory lock is broken — file a Critical backlog item. "+
			"If 0: slog phrase %q is missing from logs — probe logs above.",
		migratorCount, migratorIDs, migrationLogPhrase,
	)

	// Post-condition: verify the schema is at a real version by querying the
	// goose tracking table directly. This is the second line of defence — if
	// the log-phrase grep ever silences (e.g. log format change), the schema
	// check still catches "did migrations run at all".
	//
	// goose_db_version is the default tracking table used by pressly/goose/v3
	// (confirmed in internal/db/migrate.go comment: "goose tracks applied
	// versions in the goose_db_version table").
	db, err := sql.Open("postgres", pg.DSN)
	require.NoError(t, err, "open postgres DSN for post-condition check")
	defer db.Close()

	var maxVersion int
	err = db.QueryRowContext(ctx,
		"SELECT MAX(version_id) FROM goose_db_version WHERE is_applied = true",
	).Scan(&maxVersion)
	require.NoError(t, err,
		"goose_db_version table must exist and have applied rows after concurrent startup")
	require.Greater(t, maxVersion, 0,
		"schema must be migrated to a real version (version_id > 0)")
}
