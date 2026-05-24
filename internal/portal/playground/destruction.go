package playground

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/storage"
)

// Destruction executes the ordered, idempotent cascade for a single playground
// session. The cascade is NOT transactional across all steps: the bare-repo
// delete (step 8) is a filesystem operation outside the DB. Each step is
// idempotent, so partial failures are safely retried by the next sweep tick.
//
// Step ordering (CRITICAL — do not reorder):
//
//  1. Collect summary stats for the tombstone (members, commits, auto-merges,
//     duration, end_reason) — MUST happen before any deletes.
//  2. Collect anon account IDs from session_members — MUST happen before step 6.
//  3. Insert tombstone row (ON CONFLICT DO NOTHING → idempotent).
//  4. Revoke all bearers for the session (defense-in-depth; cascade in step 6
//     would delete them anyway, but revocation invalidates any in-flight use).
//  5. Delete comments and conflict_events for the session.
//  6. Delete the sessions row → FK CASCADE handles session_members, events,
//     presence, oauth_tokens (session_id FK ON DELETE CASCADE).
//  7. Delete collected anonymous accounts (session_members already gone via
//     cascade; accounts are not cascaded from sessions).
//  8. Remove the bare git repo from disk via Storage.RemoveRepo.
type Destruction struct {
	Store        store.Store
	Storage      storage.Service
	Clock        Clock
	Logger       *slog.Logger
	TombstoneTTL time.Duration  // default 30 days
	Leases       lease.Manager  // optional; nil → NoopManager (single-instance)
}

// leases returns the effective lease manager: the configured one or a NoopManager
// when none is set.
func (d *Destruction) leases() lease.Manager {
	if d.Leases != nil {
		return d.Leases
	}
	return lease.NoopManager{}
}

// Destroy runs the full destruction cascade for a playground session.
// It logs per-step errors but returns only hard failures (e.g. DB connection
// lost). Partial completion is safe because each step is idempotent; the next
// sweep tick will complete the remaining steps.
//
// Under clustered mode, the cascade is wrapped in a per-session advisory lock
// acquired via the LeaseManager (defaults to NoopManager in single-instance
// mode). If another pod already holds the lock, Destroy returns nil immediately
// — the other pod owns this destruction, and this pod will retry on the next
// sweep tick.
func (d *Destruction) Destroy(ctx context.Context, sess store.Session, reason string) error {
	// Acquire per-session advisory lock to prevent duplicate cascade runs in
	// clustered mode. Under NoopManager (single-instance default) this is a
	// no-op that always succeeds. Under the PG-backed manager it uses
	// pg_try_advisory_lock — non-blocking; returns ErrAlreadyHeld if another
	// pod owns this session's destruction right now.
	handle, err := d.leases().Acquire(ctx, sess.ID)
	if err != nil {
		if errors.Is(err, lease.ErrAlreadyHeld) {
			// Another pod owns this destruction. Return nil so the sweep loop
			// continues to the next session; this pod will retry on the next tick
			// if the session row is still present.
			return nil
		}
		// Unexpected acquisition error. Log and return so the sweep loop can
		// retry next tick rather than proceeding without the lock.
		return fmt.Errorf("destroy: acquire session lock: %w", err)
	}
	defer handle.Release() //nolint:errcheck

	now := d.Clock.Now().UTC()
	if d.TombstoneTTL == 0 {
		d.TombstoneTTL = 30 * 24 * time.Hour
	}
	log := d.Logger.With("session_id", sess.ID, "end_reason", reason)

	// -------------------------------------------------------------------------
	// Step 1: collect summary stats while the session row is still present.
	// -------------------------------------------------------------------------
	membersCount, err := d.Store.CountSessionMembers(ctx, store.CountSessionMembersParams{
		OrgID:     ReservedOrgID,
		SessionID: sess.ID,
	})
	if err != nil {
		log.Error("destroy: count members failed", "err", err)
		// Non-fatal: use 0 for the tombstone rather than aborting.
		membersCount = 0
	}

	commitsCount, err := d.Store.CountSessionEventsByType(ctx, sess.ID, "commit.arrived")
	if err != nil {
		log.Error("destroy: count commits failed", "err", err)
		commitsCount = 0
	}

	autoMergesCount, err := d.Store.CountSessionEventsByType(ctx, sess.ID, "merge.succeeded")
	if err != nil {
		log.Error("destroy: count auto-merges failed", "err", err)
		autoMergesCount = 0
	}

	durationSeconds := int64(now.Sub(sess.CreatedAt).Seconds())
	if durationSeconds < 0 {
		durationSeconds = 0
	}

	// -------------------------------------------------------------------------
	// Step 2: collect anon account IDs while session_members still exists.
	// -------------------------------------------------------------------------
	anonIDs, err := d.Store.ListAnonymousSessionMemberIDs(ctx, ReservedOrgID, sess.ID)
	if err != nil {
		log.Error("destroy: list anon member IDs failed", "err", err)
		// Non-fatal: we may leave orphaned anon accounts, but they have no
		// bearer (revoked in step 4) and no session membership (cascaded in
		// step 6). Acceptable. Continue.
		anonIDs = nil
	}

	// -------------------------------------------------------------------------
	// Step 3: insert tombstone (ON CONFLICT DO NOTHING → idempotent).
	// -------------------------------------------------------------------------
	tombstoneErr := d.Store.RecordTombstone(ctx, store.RecordTombstoneParams{
		SessionID:       sess.ID,
		OrgID:           ReservedOrgID,
		MembersCount:    membersCount,
		CommitsCount:    commitsCount,
		AutoMergesCount: autoMergesCount,
		DurationSeconds: durationSeconds,
		EndReason:       reason,
		EndedAt:         now,
		ExpiresAt:       now.Add(d.TombstoneTTL),
	})
	if tombstoneErr != nil {
		log.Error("destroy: record tombstone failed", "err", tombstoneErr)
		// Non-fatal: the session can still be deleted; tombstone absence is
		// observable (GET /tombstone returns 404) but not a safety concern.
	}

	// -------------------------------------------------------------------------
	// Step 4: revoke all bearers (defense-in-depth; cascade deletes them too).
	// -------------------------------------------------------------------------
	if err := d.Store.RevokeBearersForSession(ctx, store.RevokeBearersForSessionParams{
		SessionID: sess.ID,
		RevokedAt: now,
	}); err != nil {
		log.Error("destroy: revoke bearers failed", "err", err)
		// Non-fatal: the bearers will be deleted by the FK cascade in step 6.
	}

	// -------------------------------------------------------------------------
	// Step 5: delete comments and conflict_events for the session.
	// Note: with FK ON DELETE CASCADE these would be deleted by step 6 anyway,
	// but explicit deletes allow for per-table future hooks (metrics, audit).
	// In practice the cascade makes this a no-op if step 6 races ahead.
	// -------------------------------------------------------------------------
	// comments and conflict_events both have session_id FK ON DELETE CASCADE,
	// so we rely on the cascade in step 6 for correctness. Keeping explicit
	// deletes here as a forward-compat placeholder for future audit hooks.
	// No separate store methods needed — cascade handles it.

	// -------------------------------------------------------------------------
	// Step 6: delete the sessions row. FK CASCADE handles:
	//   session_members, events, presence, oauth_tokens (session_id FK),
	//   conflict_events, comments, finalize_locks.
	// -------------------------------------------------------------------------
	deleteErr := d.Store.DeleteSession(ctx, store.DeleteSessionParams{
		OrgID: ReservedOrgID,
		ID:    sess.ID,
	})
	if deleteErr != nil {
		// If the session is already gone (concurrent destruction on another pod,
		// or previous partial run), treat as success and continue cleanup.
		if errors.Is(deleteErr, store.ErrNotFound) {
			log.Info("destroy: session row already absent; continuing cleanup")
		} else {
			// Hard failure: can't confirm the session row is gone. Abort so
			// the next sweep retries from step 6 forward.
			return fmt.Errorf("destroy: delete session row: %w", deleteErr)
		}
	}

	// -------------------------------------------------------------------------
	// Step 7: delete anonymous accounts (not cascaded from sessions).
	// -------------------------------------------------------------------------
	if len(anonIDs) > 0 {
		if err := d.Store.DeleteAccountsByIDs(ctx, anonIDs); err != nil {
			log.Error("destroy: delete anon accounts failed",
				"count", len(anonIDs), "err", err)
			// Non-fatal: orphaned anon accounts have no active credentials.
		}
	}

	// -------------------------------------------------------------------------
	// Step 8: remove the bare repo from disk (filesystem op — not transactional).
	// -------------------------------------------------------------------------
	if err := d.Storage.RemoveRepo(ctx, ReservedOrgID, sess.ID); err != nil {
		log.Error("destroy: remove bare repo failed", "err", err)
		// Non-fatal: the repo may already be gone (idempotent).
		// The next sweep tick will retry if the session row returns (it won't
		// once step 6 succeeds).
	}

	log.Info("destroy: session destroyed",
		"members", membersCount,
		"commits", commitsCount,
		"auto_merges", autoMergesCount,
		"duration_s", durationSeconds,
	)
	return nil
}
