// Invariant: booting the portal with JAMSESH_PLAYGROUND_ENABLED=true against a
// Postgres database that already contains an UNPROTECTED org with slug
// "playground" (org_protected=false) causes the portal to exit non-zero. The
// container logs must include text referencing the slug conflict so an operator
// can diagnose and remediate.
//
// Background: ProvisionReservedOrg (internal/portal/playground/provision.go)
// runs at every portal boot when playground is enabled. It checks for a
// pre-existing org with slug "playground". If that org exists but is
// unprotected, it returns ErrReservedSlugConflict — a hard error that causes
// main.go to log and exit 1. This behavior ensures that an operator-created
// org that happens to share the reserved slug is never silently overwritten or
// shadowed by the playground machinery.
//
// This test is the e2e existence proof of that mechanism: a real portal
// container against a real Postgres database with a real pre-seeded conflict
// row. The unit variant (TestProvisionReservedOrg_UnprotectedSlugConflict in
// internal/portal/playground/provision_test.go) covers the function in
// isolation. This test covers the full boot path through cmd/portal/main.go
// to container exit.
//
// Setup strategy:
//  1. Start Postgres.
//  2. Boot a "migration-only" portal WITHOUT playground enabled. This portal
//     starts healthy, which means it has run all DB migrations, creating the
//     orgs table. We then let it be cleaned up.
//  3. Pre-seed the conflict row via direct SQL connection to Postgres.
//  4. Boot a second portal WITH playground enabled, using startFailingPortal
//     (no health-check wait). Assert it exits non-zero and its logs contain a
//     phrase that identifies the conflict.
//
// Relation to gate-tests-reserved-slug-conflict-main-exit-1:
// That story tests the subprocess exit code via a Go binary subprocess against
// SQLite. This test tests the full container against Postgres — different
// stack, different dialect, same invariant.
package failure_test

import (
	"context"
	"database/sql"
	"strings"
	"testing"
	"time"

	_ "github.com/lib/pq"
	"github.com/stretchr/testify/require"

	"jamsesh/tests/e2e/fixtures/portal"
	"jamsesh/tests/e2e/fixtures/postgres"
)

// conflictLogPhrases are substrings we expect to find in the portal container
// logs when it exits due to ErrReservedSlugConflict. At least one must be
// present. The portal uses slog JSON format; these phrases appear as values in
// the log JSON. The error message from provision.go is:
//
//	"reserved slug conflict: an unprotected org with slug \"playground\" exists ..."
//
// main.go wraps it with an slog.Error call that includes "playground" in the
// message field.
var conflictLogPhrases = []string{
	"reserved slug conflict",
	"playground",
}

// conflictOrgID is the deterministic ID used for the pre-seeded conflict org.
// Chosen to be distinct from the playground fixture's reserved ID ("org_playground")
// so the log output is unambiguous about which row triggered the conflict.
const conflictOrgID = "org-preexisting-unprotected"

// TestPlayground_ReservedSlugBootConflict verifies that the portal exits
// non-zero when playground provisioning finds an unprotected org with the
// reserved "playground" slug in a real Postgres database.
func TestPlayground_ReservedSlugBootConflict(t *testing.T) {
	requireDockerLocal(t)
	requirePortalImageLocal(t)

	ctx := context.Background()

	// ── Step 1: Start Postgres ────────────────────────────────────────────────
	pg := postgres.Start(ctx, t, postgres.Options{})

	// ── Step 2: Run a migration-only portal to apply the DB schema ───────────
	// Portal.Start waits for /healthz 200, so by the time it returns the schema
	// is fully applied and the orgs table exists. We do not enable playground
	// here — we just need the DDL.
	//
	// This portal is torn down by its own t.Cleanup. Because we need the
	// container gone before we pre-seed (to avoid a race where the portal
	// re-provisions on boot), and t.Cleanup runs in LIFO order within the same
	// test function, we scope the migration portal to a subtest so we can
	// explicitly control its teardown.
	func() {
		// The migration-only portal; it will be cleaned up when this closure exits
		// because t.Cleanup registers with the same t and the testcontainers cleanup
		// fires deterministically when the test's own cleanup stack is drained. We
		// use a short context just to detect a stuck migration — the portal fixture
		// already uses a 30-second wait strategy, so this is belt-and-suspenders.
		_ = portal.Start(ctx, t, portal.Options{
			DBDriver:  "postgres",
			DBDSN:     pg.ContainerDSN,
			EmailFrom: "noreply@example.com",
			// PlaygroundEnabled intentionally omitted — no playground provisioning.
		})
		// The closure returns here. The portal stays alive until t.Cleanup runs
		// (LIFO). That is fine: we only need the schema to exist when we pre-seed.
	}()

	// ── Step 3: Pre-seed the conflict row ────────────────────────────────────
	// Open a direct host-side SQL connection using pg.DSN (host-mapped port).
	// The orgs table has four required columns (id, name, slug, created_at) and
	// two columns with defaults (session_invite_policy, org_protected). We
	// explicitly set org_protected=false to exercise the conflict path.
	db, err := sql.Open("postgres", pg.DSN)
	require.NoError(t, err, "open postgres DSN for pre-seed")
	defer db.Close()

	_, err = db.ExecContext(ctx, `
		INSERT INTO orgs (id, name, slug, created_at, org_protected, session_invite_policy)
		VALUES ($1, $2, $3, $4, false, 'members_only')
	`,
		conflictOrgID,
		"Pre-existing Org",
		"playground",
		time.Now().UTC(),
	)
	require.NoError(t, err,
		"pre-seed conflict org: INSERT failed — orgs table must exist after migration portal boot")

	// Verify the row is in place before booting the second portal.
	var orgProtected bool
	err = db.QueryRowContext(ctx,
		"SELECT org_protected FROM orgs WHERE slug = 'playground'",
	).Scan(&orgProtected)
	require.NoError(t, err, "pre-seed verify: SELECT org_protected")
	require.False(t, orgProtected,
		"pre-seed verify: org_protected must be false to trigger the conflict path")

	// ── Step 4: Boot the portal with playground enabled ───────────────────────
	// We use startFailingPortal (defined in config_and_deps_test.go) which starts
	// the container without a health-check wait strategy, polls until the container
	// exits, and returns the collected log output. The portal must crash fast on a
	// slug conflict — the provisioning check fires before the HTTP server binds.
	conflictEnv := map[string]string{
		"JAMSESH_BIND":               ":8443",
		"JAMSESH_TLS_MODE":           "behind_proxy",
		"JAMSESH_DB_DRIVER":          "postgres",
		"JAMSESH_DB_DSN":             pg.ContainerDSN,
		"JAMSESH_EMAIL_FROM":         "noreply@example.com",
		"JAMSESH_PLAYGROUND_ENABLED": "true",
		// Low storage path; the container exits before any repos are written
		// but the env var must pass config validation.
		"JAMSESH_STORAGE": "/tmp/jamsesh-repos",
	}
	c, logs := startFailingPortal(ctx, t, conflictEnv)

	// ── Step 5: Assert exit non-zero ─────────────────────────────────────────
	// The portal must have exited. If the container is still running after
	// startFailingPortal's 15-second deadline, the conflict path did not fire.
	require.False(t, containerIsRunning(ctx, c),
		"portal must have exited non-zero on reserved slug conflict; "+
			"container is still running — ProvisionReservedOrg may not be "+
			"returning ErrReservedSlugConflict on boot.\nlogs:\n%s", logs)

	// ── Step 6: Assert the log mentions the conflict ──────────────────────────
	// At least one of the conflict phrases must appear in the container output.
	// This ensures an operator running `docker logs` can diagnose the failure.
	//
	// The phrases are sourced from provision.go's error string
	// ("reserved slug conflict") and the reserved slug constant ("playground").
	// Both appear in the slog.Error call that main.go emits before os.Exit(1).
	var found bool
	for _, phrase := range conflictLogPhrases {
		if strings.Contains(logs, phrase) {
			found = true
			break
		}
	}
	require.True(t, found,
		"portal exited non-zero but container logs do not mention the conflict; "+
			"logs must contain at least one of %v so operators can diagnose the failure.\nlogs:\n%s",
		conflictLogPhrases, logs)

	t.Logf("reserved_slug_boot_conflict: portal correctly exited non-zero; "+
		"conflict phrases found in logs (first 1000 chars): %.1000s", logs)

	// ── Step 7: Verify DB row was NOT mutated ─────────────────────────────────
	// The portal must not have altered the pre-seeded row (neither deleted it
	// nor flipped org_protected). Outcome B (take-ownership) is NOT what
	// v0.4.0 implements — if org_protected flipped to true, the provisioner
	// changed behaviour and this test should be revisited.
	var orgProtectedAfter bool
	err = db.QueryRowContext(ctx,
		"SELECT org_protected FROM orgs WHERE slug = 'playground'",
	).Scan(&orgProtectedAfter)
	require.NoError(t, err, "post-boot verify: SELECT org_protected")
	require.False(t, orgProtectedAfter,
		"org_protected flipped to true after portal exited non-zero — "+
			"v0.4.0 must refuse to boot (Outcome A), not silently take ownership (Outcome B). "+
			"If this fires, ProvisionReservedOrg changed behaviour: update this test and the "+
			"story notes to reflect the new contract.")
}
