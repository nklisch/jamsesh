package finalize

import "time"

// FinalizeLockTTL is the inactivity window before a held finalize lock is
// considered abandoned. Locked at epic-design (epic-finalize-flow).
//
// Every endpoint that touches a finalize-lock row performs a read-time
// idle check using IsLockExpired. No background sweeper is required.
const FinalizeLockTTL = 30 * time.Minute

// IsLockExpired reports whether a lock with the given last_activity_at is
// past the idle TTL boundary as of now. The check is strictly "past": a
// lock with last_activity_at + TTL == now is NOT expired (matches
// "<=now-TTL would mean expired" reasoning; we use strict <).
//
// Clock-skewed-backwards (lastActivity is in the future relative to now)
// returns false — we never treat a still-in-the-future activity stamp as
// expired.
func IsLockExpired(lastActivity, now time.Time) bool {
	if lastActivity.After(now) {
		return false
	}
	return now.Sub(lastActivity) > FinalizeLockTTL
}

// LockExpiresAt returns the read-time expiry timestamp for a lock with the
// supplied last_activity_at. This is the value put into the LockStatus
// and FinalizeLock response payloads — never persisted.
func LockExpiresAt(lastActivity time.Time) time.Time {
	return lastActivity.Add(FinalizeLockTTL)
}
