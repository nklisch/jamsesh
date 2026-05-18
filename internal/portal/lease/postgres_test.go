package lease_test

import (
	"context"
	"database/sql"
	"fmt"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/jackc/pgx/v5/stdlib"

	"jamsesh/internal/db"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/lease"
)

// ---------------------------------------------------------------------------
// Test harness helpers
// ---------------------------------------------------------------------------

// openPGStore opens a PG-backed Store via db.Open (which also runs migrations).
func openPGStore(t *testing.T, dsn string) store.Store {
	t.Helper()
	s, _, err := db.Open(context.Background(), "postgres", dsn, db.PoolConfig{})
	if err != nil {
		t.Fatalf("db.Open postgres: %v", err)
	}
	t.Cleanup(func() { s.Close() }) //nolint:errcheck
	return s
}

// openPGPool opens a raw pgxpool.Pool from a DSN.
func openPGPool(t *testing.T, dsn string) *pgxpool.Pool {
	t.Helper()
	pool, err := pgxpool.New(context.Background(), dsn)
	if err != nil {
		t.Fatalf("pgxpool.New: %v", err)
	}
	t.Cleanup(pool.Close)
	return pool
}

// openStdlibDB opens a *sql.DB backed by an existing pgxpool.Pool.
func openStdlibDB(t *testing.T, pool *pgxpool.Pool) *sql.DB {
	t.Helper()
	sqlDB := stdlib.OpenDBFromPool(pool)
	t.Cleanup(func() { sqlDB.Close() }) //nolint:errcheck
	return sqlDB
}

// newManager creates a PostgresManager using a fresh pool + stdlib DB.
func newManager(t *testing.T, dsn, podID string) (*lease.PostgresManager, *pgxpool.Pool) {
	t.Helper()
	s := openPGStore(t, dsn)
	pool := openPGPool(t, dsn)
	sqlDB := openStdlibDB(t, pool)
	return &lease.PostgresManager{
		DB:    sqlDB,
		Store: s,
		PodID: podID,
	}, pool
}

// cleanupLeaseRow deletes any leases row for the given session_id.
// Best-effort: test cleanup, ignore errors.
func cleanupLeaseRow(t *testing.T, dsn, sessionID string) {
	t.Helper()
	sqlDB, err := sql.Open("pgx", dsn)
	if err != nil {
		return
	}
	defer sqlDB.Close() //nolint:errcheck
	_, _ = sqlDB.ExecContext(context.Background(),
		"DELETE FROM leases WHERE session_id = $1", sessionID)
}

// uniqueSession generates a unique session_id for this test to avoid
// cross-test interference on the leases table.
func uniqueSession(t *testing.T) string {
	t.Helper()
	return fmt.Sprintf("test-ses-%s-%d", t.Name(), time.Now().UnixNano())
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestPostgresAcquireSucceeds verifies that Acquire returns a Handle with a
// non-zero FencingToken for a free session.
func TestPostgresAcquireSucceeds(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })

	mgr, _ := newManager(t, dsn, "pod-a")
	ctx := context.Background()

	h, err := mgr.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire: unexpected error: %v", err)
	}
	defer h.Release() //nolint:errcheck

	if h.SessionID() != sessionID {
		t.Errorf("SessionID() = %q; want %q", h.SessionID(), sessionID)
	}
	if tok := h.FencingToken(); tok <= 0 {
		t.Errorf("FencingToken() = %d; want > 0", tok)
	}
}

// TestPostgresFencingTokenMonotonic verifies that successive Acquire calls
// produce strictly increasing fencing tokens.
func TestPostgresFencingTokenMonotonic(t *testing.T) {
	dsn := acquireTestPostgres(t)
	ctx := context.Background()

	mgr, _ := newManager(t, dsn, "pod-a")

	var prev int64
	for i := 0; i < 3; i++ {
		sessionID := uniqueSession(t)
		t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })

		h, err := mgr.Acquire(ctx, sessionID)
		if err != nil {
			t.Fatalf("Acquire #%d: %v", i, err)
		}
		tok := h.FencingToken()
		if err := h.Release(); err != nil {
			t.Errorf("Release #%d: %v", i, err)
		}
		if tok <= prev {
			t.Errorf("token #%d = %d is not greater than previous %d", i, tok, prev)
		}
		prev = tok
	}
}

// TestPostgresAcquireConflictReturnsErrAlreadyHeld verifies that a second
// Acquire from a separate *sql.DB returns ErrAlreadyHeld while the first
// handle is still held.
func TestPostgresAcquireConflictReturnsErrAlreadyHeld(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	// First manager (pod-a) acquires the lease.
	mgr1, _ := newManager(t, dsn, "pod-a")
	h, err := mgr1.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire (pod-a): %v", err)
	}
	defer h.Release() //nolint:errcheck

	// Second manager (pod-b) uses a completely separate pool — different PG sessions.
	mgr2, _ := newManager(t, dsn, "pod-b")
	_, err = mgr2.Acquire(ctx, sessionID)
	if err == nil {
		t.Fatal("second Acquire should have returned ErrAlreadyHeld; got nil")
	}
	if err != lease.ErrAlreadyHeld {
		t.Errorf("second Acquire error = %v; want ErrAlreadyHeld", err)
	}
}

// TestPostgresHandleLostFiresOnConnKill verifies that Handle.Lost() closes
// when the holding PG backend is terminated from the outside via
// pg_terminate_backend.
func TestPostgresHandleLostFiresOnConnKill(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	// Build the manager with a fast heartbeat so we detect loss quickly.
	s := openPGStore(t, dsn)
	pool := openPGPool(t, dsn)
	sqlDB := openStdlibDB(t, pool)
	mgr := &lease.PostgresManager{
		DB:                sqlDB,
		Store:             s,
		PodID:             "pod-kill-test",
		HeartbeatInterval: 100 * time.Millisecond,
	}

	h, err := mgr.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer h.Release() //nolint:errcheck

	// Determine the backend PID of the dedicated conn the heartbeat uses.
	// We open a NEW conn on the same pool specifically to query pg_backend_pid()
	// against the holder's dedicated connection. Because the dedicated conn is
	// a separate checkout from the pool we need to find its PID via
	// pg_stat_activity. We use the advisory lock itself as a marker.
	var holderPID int
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn: %v", err)
		}
		defer adminConn.Release()

		// Query pg_stat_activity for sessions holding an advisory lock on
		// hashtext(sessionID). This finds our dedicated conn.
		err = adminConn.QueryRow(ctx, `
			SELECT pid FROM pg_stat_activity
			WHERE state != 'idle'
			  AND pid IN (
			      SELECT pid FROM pg_locks
			      WHERE locktype = 'advisory'
			        AND objid = hashtext($1)::oid
			        AND granted = true
			  )
			LIMIT 1
		`, sessionID).Scan(&holderPID)
		if err != nil {
			// Fallback: the dedicated conn may be idle between heartbeats.
			// Query all advisory lock holders for our key without state filter.
			err2 := adminConn.QueryRow(ctx, `
				SELECT pid FROM pg_locks
				WHERE locktype = 'advisory'
				  AND objid = hashtext($1)::oid
				  AND granted = true
				LIMIT 1
			`, sessionID).Scan(&holderPID)
			if err2 != nil {
				t.Fatalf("find holder PID: first err = %v; fallback err = %v", err, err2)
			}
		}
	}

	if holderPID == 0 {
		t.Fatal("could not determine holder backend PID")
	}

	// Terminate the backend holding the lease. This will cause the dedicated
	// conn's next PingContext to fail, firing Lost().
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn for terminate: %v", err)
		}
		var terminated bool
		if err := adminConn.QueryRow(ctx,
			"SELECT pg_terminate_backend($1)", holderPID,
		).Scan(&terminated); err != nil {
			adminConn.Release()
			t.Fatalf("pg_terminate_backend: %v", err)
		}
		adminConn.Release()
	}

	// Wait for Lost() to fire — should happen within a few heartbeat intervals.
	select {
	case <-h.Lost():
		// Correct: lease detected the connection drop.
	case <-time.After(3 * time.Second):
		t.Error("Lost() did not close within 3s after pg_terminate_backend")
	}
}

// TestPostgresReleaseIsIdempotent verifies that calling Release multiple times
// does not panic or return an error.
func TestPostgresReleaseIsIdempotent(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })

	mgr, _ := newManager(t, dsn, "pod-a")
	ctx := context.Background()

	h, err := mgr.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	for i := 0; i < 5; i++ {
		if err := h.Release(); err != nil {
			t.Errorf("Release() call %d returned error: %v", i+1, err)
		}
	}
}

// TestPostgresHeartbeatKeepsLeaseAlive verifies that the heartbeat goroutine
// keeps the connection alive so that Lost() does not fire during a normal
// idle period longer than the heartbeat interval.
func TestPostgresHeartbeatKeepsLeaseAlive(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	s := openPGStore(t, dsn)
	pool := openPGPool(t, dsn)
	sqlDB := openStdlibDB(t, pool)
	mgr := &lease.PostgresManager{
		DB:                sqlDB,
		Store:             s,
		PodID:             "pod-heartbeat",
		HeartbeatInterval: 100 * time.Millisecond,
	}

	h, err := mgr.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}
	defer h.Release() //nolint:errcheck

	// Sleep for 5 heartbeat intervals; Lost() must not fire.
	time.Sleep(500 * time.Millisecond)

	select {
	case <-h.Lost():
		t.Error("Lost() fired unexpectedly during heartbeat idle period")
	default:
		// Correct: lease is still held.
	}
}

// TestPostgresReleaseAfterLostIsIdempotent verifies that calling Release after
// Lost() has fired (e.g. after a connection kill) does not panic or error.
func TestPostgresReleaseAfterLostIsIdempotent(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	s := openPGStore(t, dsn)
	pool := openPGPool(t, dsn)
	sqlDB := openStdlibDB(t, pool)
	mgr := &lease.PostgresManager{
		DB:                sqlDB,
		Store:             s,
		PodID:             "pod-release-after-lost",
		HeartbeatInterval: 100 * time.Millisecond,
	}

	h, err := mgr.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Find and terminate the backend.
	var holderPID int
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn: %v", err)
		}
		err2 := adminConn.QueryRow(ctx, `
			SELECT pid FROM pg_locks
			WHERE locktype = 'advisory'
			  AND objid = hashtext($1)::oid
			  AND granted = true
			LIMIT 1
		`, sessionID).Scan(&holderPID)
		adminConn.Release()
		if err2 != nil {
			t.Fatalf("find holder PID: %v", err2)
		}
	}

	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn for terminate: %v", err)
		}
		var terminated bool
		adminConn.QueryRow(ctx, "SELECT pg_terminate_backend($1)", holderPID).Scan(&terminated) //nolint:errcheck
		adminConn.Release()
	}

	// Wait for Lost() to fire.
	select {
	case <-h.Lost():
	case <-time.After(3 * time.Second):
		t.Fatal("Lost() did not fire within 3s after terminate")
	}

	// Release after Lost — must be idempotent and error-free.
	for i := 0; i < 3; i++ {
		if err := h.Release(); err != nil {
			t.Errorf("Release() after Lost() call %d: %v", i+1, err)
		}
	}
}

// TestPostgresCollisionDefensiveCheck verifies that Acquire returns
// ErrAlreadyHeld when the leases row contains a different pod_id with
// released_at IS NULL (simulating a hashtext collision).
//
// This test pre-inserts the row directly via SQL to bypass the advisory lock
// (which the real pod would hold), then releases the advisory lock while
// keeping the row in place, and verifies that a fresh Acquire from a different
// pod returns ErrAlreadyHeld.
func TestPostgresCollisionDefensiveCheck(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	// Pre-insert a leases row as pod-original, simulating a different pod that
	// holds the lease. We insert the row without holding the advisory lock —
	// the purpose is to simulate the state AFTER a hashtext collision where
	// pod-collision has acquired the advisory lock (on a different session_id
	// that happens to hash to the same value) and the row exists for
	// sessionID with pod-original as the owner.
	//
	// To test the collision check path we need:
	//   - pod-collision to acquire the advisory lock (pg_try_advisory_lock succeeds)
	//   - the leases row to have pod_id != pod-collision AND released_at IS NULL
	//
	// We achieve this by:
	//   1. Using mgr-collision to acquire the lease (gets the lock + row with
	//      pod_id = "pod-collision").
	//   2. Manually updating the leases row to have pod_id = "pod-original"
	//      (different pod) while keeping released_at NULL.
	//   3. Releasing the advisory lock (via pg_advisory_unlock) but NOT the
	//      leases row — simulating the split-brain window.
	//   4. Attempting Acquire from pod-collision again — it will get the advisory
	//      lock (it was released), see a row with pod_id = "pod-original", and
	//      return ErrAlreadyHeld.

	// Step 1: acquire with pod-collision to create the row.
	mgr1, pool := newManager(t, dsn, "pod-collision")
	h1, err := mgr1.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("initial Acquire: %v", err)
	}

	// Step 2: tamper — update the row's pod_id to "pod-original" while holding
	// the lock. We use a separate admin connection.
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn: %v", err)
		}
		_, err = adminConn.Exec(ctx,
			"UPDATE leases SET pod_id = 'pod-original', released_at = NULL WHERE session_id = $1",
			sessionID)
		adminConn.Release()
		if err != nil {
			h1.Release() //nolint:errcheck
			t.Fatalf("tamper leases row: %v", err)
		}
	}

	// Step 3: release the advisory lock from the handle (but the row still has
	// pod_id = "pod-original"). We call Release which also calls
	// pg_advisory_unlock and MarkLeaseReleased — but MarkLeaseReleased sets
	// released_at = now(). We need released_at to remain NULL so the collision
	// check triggers. Override: release the lock manually first, then skip
	// the MarkLeaseReleased effect by re-nulling released_at.
	//
	// Simpler approach: just call h1.Release() (which marks released_at),
	// then re-null it in the admin conn. The collision check only fires when
	// released_at IS NULL.
	if err := h1.Release(); err != nil {
		t.Fatalf("Release h1: %v", err)
	}
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn: %v", err)
		}
		_, err = adminConn.Exec(ctx,
			"UPDATE leases SET released_at = NULL WHERE session_id = $1", sessionID)
		adminConn.Release()
		if err != nil {
			t.Fatalf("re-null released_at: %v", err)
		}
	}

	// Step 4: try Acquire with pod-collision — advisory lock is free (released
	// by h1.Release), but the leases row has pod_id = "pod-original" with
	// released_at = NULL. The collision check must return ErrAlreadyHeld.
	_, err = mgr1.Acquire(ctx, sessionID)
	if err == nil {
		t.Fatal("Acquire should have returned ErrAlreadyHeld for collision; got nil")
	}
	if err != lease.ErrAlreadyHeld {
		t.Errorf("Acquire error = %v; want ErrAlreadyHeld", err)
	}
}
