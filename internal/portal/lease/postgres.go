package lease

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"log/slog"
	"sync"
	"time"

	"jamsesh/internal/db/store"
)

const defaultHeartbeatInterval = 10 * time.Second

// PostgresManager is the production lease implementation. Each successful
// Acquire checks out a dedicated *sql.Conn from the pool for the lease's
// entire lifetime so that the PG advisory lock (which is session-scoped)
// stays attributed to the correct PG backend. Release returns the conn to
// the pool.
//
// DB must be a *sql.DB backed by pgxpool (opened via pgx/v5/stdlib). Store
// must be a PG-backed store.Store (the SQLite adapter returns errors for
// IssueLeaseFencingToken and the sequence-based advisory-lock path).
type PostgresManager struct {
	DB                *sql.DB        // pgxpool-backed *sql.DB
	Store             store.Store    // for InsertLease / IssueLeaseFencingToken
	PodID             string         // identifies this pod in the leases table
	HeartbeatInterval time.Duration  // default 10s when zero
}

// heartbeatInterval returns the configured interval or the default.
func (m *PostgresManager) heartbeatInterval() time.Duration {
	if m.HeartbeatInterval > 0 {
		return m.HeartbeatInterval
	}
	return defaultHeartbeatInterval
}

// Acquire attempts a non-blocking lease acquisition for sessionID.
//
// Sequence:
//  1. Check out a dedicated *sql.Conn (advisory locks are session-scoped).
//  2. SELECT pg_try_advisory_lock(hashtext($session_id)) — false → ErrAlreadyHeld.
//  3. Issue a fencing token via the jamsesh_lease_fencing_tokens sequence.
//  4. Upsert the leases row.
//  5. Defensive hashtext-collision check: if the leases row has a different
//     pod_id and released_at IS NULL, release the lock and return ErrAlreadyHeld.
//  6. Spawn heartbeat goroutine; return *pgHandle.
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
		return nil, fmt.Errorf("lease: pg_try_advisory_lock: %w", err)
	}
	if !acquired {
		conn.Close() //nolint:errcheck
		return nil, ErrAlreadyHeld
	}

	// Step 3: issue fencing token.
	fencingToken, err := m.Store.IssueLeaseFencingToken(ctx)
	if err != nil {
		// Best-effort advisory unlock before returning.
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
		conn.Close() //nolint:errcheck
		return nil, fmt.Errorf("lease: issue fencing token: %w", err)
	}

	// Step 4: upsert the leases row.
	if _, err := m.Store.InsertLease(ctx, store.InsertLeaseParams{
		SessionID:    sessionID,
		PodID:        m.PodID,
		FencingToken: fencingToken,
	}); err != nil {
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
		conn.Close() //nolint:errcheck
		return nil, fmt.Errorf("lease: upsert lease row: %w", err)
	}

	// Step 5: hashtext collision defensive check.
	// After acquiring the advisory lock, read back the row and confirm the
	// pod_id matches us. A different pod_id with released_at IS NULL means
	// two different session_id strings happened to hash to the same int32 —
	// extremely rare but documented.
	var rowPodID string
	var rowReleasedAt *time.Time
	err = conn.QueryRowContext(ctx,
		"SELECT pod_id, released_at FROM leases WHERE session_id = $1", sessionID,
	).Scan(&rowPodID, &rowReleasedAt)
	if err != nil && !errors.Is(err, sql.ErrNoRows) {
		// Couldn't verify — conservative fail.
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
		conn.Close() //nolint:errcheck
		return nil, fmt.Errorf("lease: collision check query: %w", err)
	}
	if rowPodID != m.PodID && rowReleasedAt == nil {
		slog.Warn("lease: hashtext collision detected; releasing advisory lock",
			"session_id", sessionID,
			"our_pod_id", m.PodID,
			"holding_pod_id", rowPodID,
		)
		_, _ = conn.ExecContext(ctx, "SELECT pg_advisory_unlock(hashtext($1))", sessionID)
		conn.Close() //nolint:errcheck
		return nil, ErrAlreadyHeld
	}

	// Step 6: build the handle and start heartbeat.
	h := &pgHandle{
		sessionID:    sessionID,
		fencingToken: fencingToken,
		conn:         conn,
		store:        m.Store,
		lost:         make(chan struct{}),
		done:         make(chan struct{}),
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
	conn         *sql.Conn
	store        store.Store

	lost chan struct{} // closed when the lease is lost (heartbeat failure)
	done chan struct{} // closed by Release to stop the heartbeat goroutine

	once sync.Once // guards the release sequence
}

func (h *pgHandle) SessionID() string        { return h.sessionID }
func (h *pgHandle) FencingToken() int64      { return h.fencingToken }
func (h *pgHandle) Lost() <-chan struct{}     { return h.lost }

// Release relinquishes the lease. Idempotent — safe to call multiple times and
// safe to call after Lost() has fired. The sequence is:
//  1. Signal the heartbeat goroutine to stop (via done channel).
//  2. Advisory unlock (best-effort; the conn may already be dead if Lost fired).
//  3. Mark the leases row released_at = now() (best-effort).
//  4. Close the dedicated conn (returns it to the pool).
func (h *pgHandle) Release() error {
	h.once.Do(func() {
		// Signal the heartbeat goroutine to stop.
		close(h.done)

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
func (h *pgHandle) runHeartbeat(interval time.Duration) {
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
				// Connection lost — close Lost() exactly once.
				select {
				case <-h.lost:
					// Already closed by a previous failure (shouldn't happen with
					// a single goroutine, but be defensive).
				default:
					close(h.lost)
				}
				return
			}
		}
	}
}
