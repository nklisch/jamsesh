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

// TestPostgresReclaimsStaleSameSessionRow verifies that a survivor pod takes
// over a lease whose previous holder exited without clearing released_at — the
// SIGKILL / ungraceful-drain case. After the holder's PG connection dies, its
// session-scoped advisory lock auto-releases, but the leases row lingers with
// pod_id = <dead holder> and released_at IS NULL. A survivor that wins
// pg_try_advisory_lock MUST reclaim that row (overwrite pod_id + fencing_token,
// re-null released_at) rather than refuse the takeover.
//
// TEST-DEBT NOTE: an earlier test (TestPostgresCollisionDefensiveCheck) set up
// this exact state — same session_id, advisory lock free, a different pod_id
// with released_at IS NULL — and asserted ErrAlreadyHeld, calling it a "hashtext
// collision". That assertion ENCODED THE BUG (feature
// e2e-cloud-native-multipod-suite-red-lease-migration): a same-session row can
// never prove a cross-session hashtext collision (the colliding row would carry
// a DIFFERENT session_id), and a true collision is already excluded at step 2
// because both colliding sessions contend on the same advisory-lock key. The
// false positive made a survivor unable to ever take over a dead holder's lease.
// This test now asserts the corrected takeover behavior.
func TestPostgresReclaimsStaleSameSessionRow(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	// Build the stale dead-holder state:
	//   1. mgr-dead acquires the lease (advisory lock + row, pod_id = "pod-dead").
	//   2. Stamp the row pod_id = "pod-dead" with released_at = NULL (it already
	//      is, but make the intent explicit) — this is what a holder that died
	//      ungracefully leaves behind.
	//   3. Release the advisory lock WITHOUT marking released_at, mirroring a
	//      connection-death auto-release. (h.Release() marks released_at = now(),
	//      so we re-null it afterwards to reproduce the SIGKILL residue.)
	mgrDead, pool := newManager(t, dsn, "pod-dead")
	hDead, err := mgrDead.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("initial Acquire (pod-dead): %v", err)
	}
	deadToken := hDead.FencingToken()

	// Release frees the advisory lock (as a connection death would) but also
	// marks released_at = now(); re-null it so the row looks like an unclean exit.
	if err := hDead.Release(); err != nil {
		t.Fatalf("Release hDead: %v", err)
	}
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn: %v", err)
		}
		_, err = adminConn.Exec(ctx,
			"UPDATE leases SET pod_id = 'pod-dead', released_at = NULL WHERE session_id = $1",
			sessionID)
		adminConn.Release()
		if err != nil {
			t.Fatalf("reproduce unclean-exit row: %v", err)
		}
	}

	// A survivor with a DIFFERENT pod_id wins the advisory lock (it is free) and
	// must reclaim the stale row — not return ErrAlreadyHeld.
	mgrSurvivor, _ := newManager(t, dsn, "pod-survivor")
	hSurvivor, err := mgrSurvivor.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("survivor Acquire should reclaim stale dead-holder lease; got error: %v", err)
	}
	defer hSurvivor.Release() //nolint:errcheck

	// The takeover must mint a fresh, strictly greater fencing token.
	if got := hSurvivor.FencingToken(); got <= deadToken {
		t.Errorf("survivor FencingToken() = %d; want > dead holder's token %d", got, deadToken)
	}

	// The row must now be owned by the survivor with released_at re-nulled.
	{
		adminConn, err := pool.Acquire(ctx)
		if err != nil {
			t.Fatalf("acquire admin conn (verify): %v", err)
		}
		var podID string
		var releasedAt *time.Time
		err = adminConn.QueryRow(ctx,
			"SELECT pod_id, released_at FROM leases WHERE session_id = $1", sessionID,
		).Scan(&podID, &releasedAt)
		adminConn.Release()
		if err != nil {
			t.Fatalf("verify reclaimed row: %v", err)
		}
		if podID != "pod-survivor" {
			t.Errorf("reclaimed row pod_id = %q; want %q", podID, "pod-survivor")
		}
		if releasedAt != nil {
			t.Errorf("reclaimed row released_at = %v; want NULL (active)", releasedAt)
		}
	}
}

// TestPostgresHeartbeatConnRace verifies that Release does not race the
// heartbeat goroutine on the shared *sql.Conn. The test acquires a lease with
// a very short heartbeat interval so the ping fires frequently, then calls
// Release while the heartbeat is likely mid-tick. With -race this detects any
// concurrent use of the *sql.Conn between the two goroutines.
//
// This is the regression test for bug-squash-pghandle-heartbeat-conn-race.
// Requires a Postgres testcontainer (skipped if Docker is unavailable).
func TestPostgresHeartbeatConnRace(t *testing.T) {
	dsn := acquireTestPostgres(t)
	sessionID := uniqueSession(t)
	t.Cleanup(func() { cleanupLeaseRow(t, dsn, sessionID) })
	ctx := context.Background()

	s := openPGStore(t, dsn)
	pool := openPGPool(t, dsn)
	sqlDB := openStdlibDB(t, pool)

	// Very short heartbeat so the ping goroutine ticks while we call Release.
	mgr := &lease.PostgresManager{
		DB:                sqlDB,
		Store:             s,
		PodID:             "pod-race-test",
		HeartbeatInterval: 5 * time.Millisecond,
	}

	h, err := mgr.Acquire(ctx, sessionID)
	if err != nil {
		t.Fatalf("Acquire: %v", err)
	}

	// Let the heartbeat tick several times so there is a good chance we call
	// Release while a PingContext is in flight.
	time.Sleep(30 * time.Millisecond)

	// Release must return cleanly. Under -race, any concurrent use of the
	// *sql.Conn by runHeartbeat and Release will be reported here.
	if err := h.Release(); err != nil {
		t.Errorf("Release: %v", err)
	}

	// Verify idempotency — a second Release must not panic or error.
	if err := h.Release(); err != nil {
		t.Errorf("second Release: %v", err)
	}
}
