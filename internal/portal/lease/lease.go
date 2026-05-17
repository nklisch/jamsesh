// Package lease provides session lease primitives for the portal.
//
// In clustered mode a [Manager] backed by Postgres advisory locks enforces
// mutual exclusion across pods — at most one pod holds the lease for any
// given session_id at a time. In single-instance mode [NoopManager] is
// used instead; it always succeeds, never blocks, and carries a fencing
// token of 0 (the documented "no fencing required" sentinel).
//
// Consumers should select on [Handle.Lost] alongside their request contexts
// so that a lease loss (PG session drop, heartbeat failure) is detected and
// the in-flight request can be aborted before writing stale data.
package lease

import (
	"context"
	"errors"
)

// ErrAlreadyHeld is returned by [Manager.Acquire] when another pod (PG
// session) currently holds the lease for the requested session_id.
var ErrAlreadyHeld = errors.New("lease: session lease already held by another pod")

// Manager creates leases for portal sessions. The Postgres implementation
// uses per-session advisory locks to enforce mutual exclusion across pods;
// [NoopManager] is the single-instance shim that always succeeds.
type Manager interface {
	// Acquire attempts a non-blocking lease acquisition for sessionID.
	// Returns [ErrAlreadyHeld] immediately if another pod holds the lock.
	//
	// The returned [Handle] owns a dedicated Postgres connection for the
	// duration of the lease (Postgres impl only); Release MUST be called to
	// free it. Callers should use defer h.Release() immediately after a
	// successful Acquire.
	Acquire(ctx context.Context, sessionID string) (Handle, error)
}

// Handle is an active lease. Consumers (object-storage-sync,
// hydration-handoff) inspect [Handle.FencingToken] on every guarded
// operation and monitor [Handle.Lost] to abort serving when the lease is
// gone.
type Handle interface {
	// SessionID returns the session identifier this Handle was acquired for.
	SessionID() string

	// FencingToken returns the monotonically increasing token issued at
	// acquisition time. Downstream writers pass this token on every
	// object-storage or cache operation so that a new lease holder can
	// reject writes from a previous, stale holder.
	//
	// A token of 0 is the documented "no fencing required" sentinel used
	// by [NoopManager] in single-instance mode.
	FencingToken() int64

	// Lost returns a channel that closes when the lease is lost — for
	// example because the underlying Postgres session died or the heartbeat
	// goroutine detected a connection error. Consumers should select on
	// this alongside their request contexts and abort serving the session
	// immediately when it fires.
	//
	// Lost() never fires before [Handle.Release] is called on a
	// [NoopManager] handle.
	Lost() <-chan struct{}

	// Release relinquishes the lease and frees any underlying resources
	// (e.g. the dedicated Postgres connection). Idempotent — safe to call
	// multiple times and safe to call after Lost() has fired.
	Release() error
}
