package portalcluster

import (
	"context"
	"database/sql"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// RequireLeaseHolder polls LeaseHolder until a holder is found or retryTimeout
// elapses. Returns the pod index (>= 0) of the holder on success.
//
// On timeout the test is fatally failed — RequireLeaseHolder is the
// require-style variant of LeaseHolder intended for tests that have already
// confirmed the lease must be held (e.g. after a successful API call that
// triggers acquisition). If the answer is "we don't know yet", use
// LeaseHolder directly.
func (c *Cluster) RequireLeaseHolder(ctx context.Context, t *testing.T, sessionID string, retryTimeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(retryTimeout)
	for time.Now().Before(deadline) {
		if h := c.LeaseHolder(ctx, t, sessionID); h >= 0 {
			return h
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("RequireLeaseHolder: no pod holds lease for %q after %v", sessionID, retryTimeout)
	return -1 // unreachable; satisfies compiler
}

// FencingTokenForSession queries the leases table for the most recent
// fencing_token issued for sessionID. Returns -1 if no row exists.
//
// The token is read from the leases table written by PostgresManager.Acquire
// (via InsertLease). The table is backed by the jamsesh_lease_fencing_tokens
// sequence so tokens are always > 0 for an active PostgresManager. A return
// of 0 from a clustered portal indicates a bug — the NoopManager sentinel
// only appears in single-instance mode and the clustered-mode e2e tests
// must assert token > 0.
//
// On query error the test is fatally failed. On sql.ErrNoRows (no lease row
// for sessionID) the method returns -1 so callers can distinguish "never
// acquired" from a token value of 0.
func (c *Cluster) FencingTokenForSession(ctx context.Context, t *testing.T, sessionID string) int64 {
	t.Helper()

	db, err := sql.Open("postgres", c.postgres.DSN)
	if err != nil {
		t.Fatalf("FencingTokenForSession: open DB: %v", err)
	}
	defer db.Close()

	// ORDER BY acquired_at DESC LIMIT 1 picks the most recently upserted row.
	// InsertLease uses ON CONFLICT(session_id) DO UPDATE, so there is at most
	// one row per session_id; the ORDER BY is defensive against a future
	// schema that partitions by acquisition rather than upserts.
	var token int64
	err = db.QueryRowContext(ctx,
		"SELECT fencing_token FROM leases WHERE session_id = $1 ORDER BY acquired_at DESC LIMIT 1",
		sessionID,
	).Scan(&token)
	if err == sql.ErrNoRows {
		return -1
	}
	if err != nil {
		t.Fatalf("FencingTokenForSession: query: %v", err)
	}
	return token
}

// ReleaseLeaseForcibly marks the most recent lease row as released in Postgres
// without going through the portal API or advisory-lock protocol. Used by
// stale-token tests to simulate re-acquisition: after calling this, the portal
// can insert a new row with a higher fencing token, producing a stale-vs-fresh
// token pair suitable for rejection testing.
//
// IMPORTANT: this method manipulates Postgres state only. It does NOT release
// the Postgres advisory lock held by the portal's dedicated connection.
// Advisory locks are released automatically when the connection closes (e.g.
// after Kill). Call ReleaseLeaseForcibly AFTER Kill to ensure the advisory
// lock is already gone; otherwise the table row and the lock are out of sync,
// which confuses the collision-detection check in PostgresManager.Acquire.
//
// On query error the test is fatally failed.
func (c *Cluster) ReleaseLeaseForcibly(ctx context.Context, t *testing.T, sessionID string) {
	t.Helper()

	db, err := sql.Open("postgres", c.postgres.DSN)
	if err != nil {
		t.Fatalf("ReleaseLeaseForcibly: open DB: %v", err)
	}
	defer db.Close()

	result, err := db.ExecContext(ctx,
		"UPDATE leases SET released_at = now() WHERE session_id = $1 AND released_at IS NULL",
		sessionID,
	)
	if err != nil {
		t.Fatalf("ReleaseLeaseForcibly: update leases: %v", err)
	}

	rows, err := result.RowsAffected()
	if err != nil {
		// RowsAffected errors are driver-specific; log and continue rather than
		// fatal — the UPDATE either succeeded or would have fataled above.
		t.Logf("ReleaseLeaseForcibly: RowsAffected: %v (non-fatal)", err)
		return
	}
	if rows == 0 {
		// No unreleased row found for sessionID — this is almost certainly a
		// test-design error (calling ReleaseLeaseForcibly before a lease was
		// acquired, or calling it twice). Log loudly but don't fatal: the
		// downstream test's assertion on fencing-token values will surface the
		// real problem with better context.
		t.Logf("ReleaseLeaseForcibly: WARNING: no unreleased lease row found for session %q — was the lease acquired?", sessionID)
	}
}
