package lease

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/metrics"
)

const defaultHeartbeatInterval = 10 * time.Second

// PostgresManager is the production lease implementation. Each successful
// Acquire checks out a dedicated *sql.Conn from the pool for the lease's
// entire lifetime so that the PG advisory lock (which is session-scoped)
// stays attributed to the correct PG backend. Release returns the conn to
// the pool.
//
// DB must be a *sql.DB backed by pgxpool (opened via pgx/v5/stdlib). Store
// must be a PG-backed store.LeaseStore (the SQLite adapter returns errors for
// IssueLeaseFencingToken and the sequence-based advisory-lock path).
type PostgresManager struct {
	DB                *sql.DB           // pgxpool-backed *sql.DB
	Store             store.LeaseStore  // for InsertLease / IssueLeaseFencingToken
	PodID             string            // identifies this pod in the leases table
	HeartbeatInterval time.Duration     // default 10s when zero
	Metrics           *metrics.Registry // optional; nil disables metrics emission
}

// heartbeatInterval returns the configured interval or the default.
func (m *PostgresManager) heartbeatInterval() time.Duration {
	if m.HeartbeatInterval > 0 {
		return m.HeartbeatInterval
	}
	return defaultHeartbeatInterval
}

// incAcquires increments the lease acquire counter with the given result label.
// Nil-safe: no-op when m.Metrics is nil.
func (m *PostgresManager) incAcquires(result string) {
	if m.Metrics != nil {
		m.Metrics.LeaseAcquiresTotal.WithLabelValues(result).Inc()
	}
}

// incHolds adjusts the current-holds gauge by delta (typically +1 or -1).
// Nil-safe: no-op when m.Metrics is nil.
func (m *PostgresManager) incHolds(delta float64) {
	if m.Metrics != nil {
		m.Metrics.LeaseHoldsCurrently.Add(delta)
	}
}

// incFencingTokens increments the fencing-tokens-issued counter.
// Nil-safe: no-op when m.Metrics is nil.
func (m *PostgresManager) incFencingTokens() {
	if m.Metrics != nil {
		m.Metrics.LeaseFencingTokensIssuedTotal.Inc()
	}
}

// incLost increments the lease-lost counter.
// Nil-safe: no-op when m.Metrics is nil.
func (m *PostgresManager) incLost() {
	if m.Metrics != nil {
		m.Metrics.LeaseLostTotal.Inc()
	}
}

// observeHoldDuration records a hold-duration observation in seconds.
// Nil-safe: no-op when m.Metrics is nil.
func (m *PostgresManager) observeHoldDuration(d time.Duration) {
	if m.Metrics != nil {
		m.Metrics.LeaseHoldDurationSeconds.Observe(d.Seconds())
	}
}

// Acquire attempts a non-blocking lease acquisition for sessionID.
//
// Sequence:
//  1. Check out a dedicated *sql.Conn (advisory locks are session-scoped).
//  2. SELECT pg_try_advisory_lock(hashtext($session_id)) — false → ErrAlreadyHeld.
//     This is the ONLY contention gate, and it covers both cases that block a
//     takeover: (a) a live holder of THIS session still owns the lock, and
//     (b) a true cross-session hashtext collision — a DIFFERENT session_id that
//     hashes to the same int32 — because both colliding sessions contend on the
//     same advisory-lock key, so the second caller's try fails.
//  3. Issue a fencing token via the jamsesh_lease_fencing_tokens sequence.
//  4. Upsert the leases row (ON CONFLICT DO UPDATE) — this completes a takeover
//     by overwriting pod_id / fencing_token and re-nulling released_at.
//  5. Spawn heartbeat goroutine; return *pgHandle.
//
// Reaching step 3 means we hold the advisory lock, which proves there is no live
// holder. Any leftover same-session row with released_at IS NULL is therefore a
// STALE row from a holder that exited without clearing released_at (SIGKILL,
// connection drop, ungraceful drain) — its session-scoped lock auto-released on
// connection death. That row is reclaimable, and the step-4 upsert reclaims it.
// We deliberately do NOT inspect the existing row to "detect a collision": a
// same-session row can never prove a cross-session collision (the colliding row
// would carry a different session_id), and a true collision is already excluded
// at step 2. An earlier version read the row here and refused takeover whenever
// pod_id differed and released_at was NULL — that was a false positive that made
// a survivor unable to ever take over a dead holder's lease.
func (m *PostgresManager) Acquire(ctx context.Context, sessionID string) (Handle, error) {
	// Step 1: dedicate a connection for this lease's lifetime.
	conn, err := m.DB.Conn(ctx)
	if err != nil {
		return nil, fmt.Errorf("lease: checkout dedicated conn: %w", err)
	}

	// Step 2: try the advisory lock.
	var acquired bool
	if err := conn.QueryRowContext(ctx,
		"SELECT pg_try_advisory_lock(hashtext($1))", sessionID,
	).Scan(&acquired); err != nil {
		conn.Close() //nolint:errcheck
		m.incAcquires("error")
		return nil, fmt.Errorf("lease: pg_try_advisory_lock: %w", err)
	}
	if !acquired {
		conn.Close() //nolint:errcheck
		m.incAcquires("conflict")
		return nil, ErrAlreadyHeld
	}

	// We now hold the advisory lock, so there is no live holder for this session
	// (the lock is session-scoped and auto-releases when the holder's PG
	// connection dies). A leftover same-session row with released_at IS NULL is a
	// stale dead-holder row; the step-4 upsert reclaims it. See the Acquire doc
	// comment for why we do not (and cannot) treat that row as a collision here.

	// Step 3: issue fencing token.
	fencingToken, err := m.Store.IssueLeaseFencingToken(ctx)
	if err != nil {
		// Best-effort advisory unlock before returning.
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
		conn.Close() //nolint:errcheck
		m.incAcquires("error")
		return nil, fmt.Errorf("lease: issue fencing token: %w", err)
	}

	// Step 4: upsert the leases row (reclaims any stale same-session row).
	if _, err := m.Store.InsertLease(ctx, store.InsertLeaseParams{
		SessionID:    sessionID,
		PodID:        m.PodID,
		FencingToken: fencingToken,
	}); err != nil {
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
		conn.Close() //nolint:errcheck
		m.incAcquires("error")
		return nil, fmt.Errorf("lease: upsert lease row: %w", err)
	}

	// Step 5: emit success metrics and build the handle.
	m.incAcquires("ok")
	m.incFencingTokens()
	m.incHolds(+1)

	h := &pgHandle{
		sessionID:     sessionID,
		fencingToken:  fencingToken,
		acquiredAt:    time.Now(),
		conn:          conn,
		store:         m.Store,
		mgr:           m,
		lost:          make(chan struct{}),
		done:          make(chan struct{}),
		heartbeatDone: make(chan struct{}),
	}
	go h.runHeartbeat(m.heartbeatInterval())

	return h, nil
}

// ---------------------------------------------------------------------------
// pgHandle
// ---------------------------------------------------------------------------

// pgHandle is the Handle returned by PostgresManager.Acquire. It owns the
// dedicated *sql.Conn for the lease's lifetime.
type pgHandle struct {
	sessionID    string
	fencingToken int64
	acquiredAt   time.Time        // used to compute hold duration on Release
	conn         *sql.Conn
	store        store.LeaseStore
	mgr          *PostgresManager // back-reference for metric emission

	lost          chan struct{} // closed when the lease is lost (heartbeat failure)
	done          chan struct{} // closed by Release to stop the heartbeat goroutine
	heartbeatDone chan struct{} // closed by runHeartbeat on exit; Release waits before touching conn

	once sync.Once // guards the release sequence
}

func (h *pgHandle) SessionID() string        { return h.sessionID }
func (h *pgHandle) FencingToken() int64      { return h.fencingToken }
func (h *pgHandle) Lost() <-chan struct{}     { return h.lost }

// Release relinquishes the lease. Idempotent — safe to call multiple times and
// safe to call after Lost() has fired. The sequence is:
//  1. Signal the heartbeat goroutine to stop (via done channel).
//  2. Wait for the heartbeat goroutine to exit (via heartbeatDone channel).
//     This ensures we are the sole user of h.conn before touching it;
//     database/sql forbids concurrent use of a single *sql.Conn.
//     The wait is bounded by one ping-context timeout (= heartbeat interval).
//  3. Advisory unlock (best-effort; the conn may already be dead if Lost fired).
//  4. Mark the leases row released_at = now() (best-effort).
//  5. Close the dedicated conn (returns it to the pool).
func (h *pgHandle) Release() error {
	h.once.Do(func() {
		// Signal the heartbeat goroutine to stop.
		close(h.done)

		// Wait for the heartbeat goroutine to finish. After close(h.done) the
		// goroutine exits either immediately (if blocked in select) or after
		// completing an in-flight PingContext (bounded by the ping ctx timeout =
		// one heartbeat interval). We must be the sole owner of h.conn before
		// proceeding — database/sql does not allow concurrent use of one *sql.Conn.
		<-h.heartbeatDone

		// Emit metrics: decrement active holds and observe hold duration.
		if h.mgr != nil {
			h.mgr.incHolds(-1)
			h.mgr.observeHoldDuration(time.Since(h.acquiredAt))
		}

		// Advisory unlock — best effort; if the conn is dead this will fail
		// silently, which is fine: the PG session drop already released the lock.
		ctx := context.Background()
		_, _ = h.conn.ExecContext(ctx,
			"SELECT pg_advisory_unlock(hashtext($1))", h.sessionID)

		// Mark the row released — best effort.
		_ = h.store.MarkLeaseReleased(ctx, h.sessionID)

		// Return the conn to the pool.
		h.conn.Close() //nolint:errcheck
	})
	return nil
}

// runHeartbeat pings the dedicated connection every interval. Any ping failure
// closes Lost() so that consumers abort serving the session. The goroutine
// exits either when done is closed (Release called) or when the ping fails.
// On exit it closes heartbeatDone to unblock Release's wait.
func (h *pgHandle) runHeartbeat(interval time.Duration) {
	defer close(h.heartbeatDone) // signal Release that we've exited and h.conn is safe to use

	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-h.done:
			// Release was called; stop the goroutine without firing Lost.
			return
		case <-ticker.C:
			ctx, cancel := context.WithTimeout(context.Background(), interval)
			err := h.conn.PingContext(ctx)
			cancel()
			if err != nil {
				// Connection lost — close Lost() exactly once and emit metric.
				select {
				case <-h.lost:
					// Already closed by a previous failure (shouldn't happen with
					// a single goroutine, but be defensive).
				default:
					close(h.lost)
					if h.mgr != nil {
						h.mgr.incLost()
					}
				}
				return
			}
		}
	}
}
