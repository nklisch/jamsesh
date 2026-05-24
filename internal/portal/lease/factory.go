package lease

import (
	"database/sql"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/metrics"
)

// New returns the Manager appropriate for the configured deploy mode.
//
//   - deployMode "single" (or any value other than "clustered") → [NoopManager].
//   - deployMode "clustered" → [PostgresManager] backed by db and the provided Store.
//
// heartbeatInterval is passed directly to [PostgresManager.HeartbeatInterval]; the
// zero value falls back to the 10-second default. The optional metricsReg is wired
// into the [PostgresManager] for nil-safe metric emission; pass nil to disable.
func New(
	deployMode string,
	db *sql.DB,
	s store.LeaseStore,
	podID string,
	heartbeatInterval time.Duration,
	metricsReg *metrics.Registry,
) Manager {
	if deployMode != "clustered" {
		return NoopManager{}
	}
	return &PostgresManager{
		DB:                db,
		Store:             s,
		PodID:             podID,
		HeartbeatInterval: heartbeatInterval,
		Metrics:           metricsReg,
	}
}
