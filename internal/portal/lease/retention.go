package lease

import (
	"context"
	"log/slog"
	"time"

	"jamsesh/internal/db/store"
)

// RunRetention periodically deletes released lease rows that are older than
// retentionAfter. It uses [store.Store.DeleteReleasedLeasesOlderThan] and
// runs on the provided interval.
//
// The function blocks until ctx is cancelled (e.g. on SIGTERM). It is safe
// to run as a goroutine. For single-instance deployments (where [NoopManager]
// is used and no lease rows are ever written), the underlying query is a no-op
// and RunRetention is not called at all — the caller (main.go) only starts the
// goroutine in clustered mode.
func RunRetention(ctx context.Context, s store.Store, interval, retentionAfter time.Duration) error {
	ticker := time.NewTicker(interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-ticker.C:
			cutoff := time.Now().Add(-retentionAfter)
			if err := s.DeleteReleasedLeasesOlderThan(ctx, cutoff); err != nil {
				// Log and continue — a transient DB error should not crash the pod.
				slog.Warn("lease retention: failed to delete old lease rows",
					"err", err,
					"cutoff", cutoff,
				)
			} else {
				slog.Debug("lease retention: deleted old released lease rows",
					"cutoff", cutoff,
				)
			}
		}
	}
}
