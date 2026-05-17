package lease_test

import (
	"testing"
	"time"

	"jamsesh/internal/portal/lease"
)

// TestNew_SingleMode verifies that lease.New returns a NoopManager for the
// "single" deploy mode (the default).
func TestNew_SingleMode(t *testing.T) {
	mgr := lease.New("single", nil, nil, "pod-1", 10*time.Second, nil)
	if _, ok := mgr.(lease.NoopManager); !ok {
		t.Errorf("New(\"single\") returned %T, want lease.NoopManager", mgr)
	}
}

// TestNew_UnknownMode verifies that any value other than "clustered" falls back
// to NoopManager — unknown modes must not panic at startup.
func TestNew_UnknownMode(t *testing.T) {
	for _, mode := range []string{"", "SINGLE", "none", "distributed"} {
		t.Run("mode="+mode, func(t *testing.T) {
			mgr := lease.New(mode, nil, nil, "pod-1", 10*time.Second, nil)
			if _, ok := mgr.(lease.NoopManager); !ok {
				t.Errorf("New(%q) returned %T, want lease.NoopManager", mode, mgr)
			}
		})
	}
}

// TestNew_ClusteredMode verifies that lease.New returns a *PostgresManager for
// the "clustered" deploy mode with the expected fields set.
// We pass nil for db and store because the factory only assigns fields — no I/O.
func TestNew_ClusteredMode(t *testing.T) {
	mgr := lease.New("clustered", nil, nil, "pod-cluster-1", 15*time.Second, nil)
	pm, ok := mgr.(*lease.PostgresManager)
	if !ok {
		t.Fatalf("New(\"clustered\") returned %T, want *lease.PostgresManager", mgr)
	}
	if pm.PodID != "pod-cluster-1" {
		t.Errorf("PostgresManager.PodID = %q, want %q", pm.PodID, "pod-cluster-1")
	}
	if pm.HeartbeatInterval != 15*time.Second {
		t.Errorf("PostgresManager.HeartbeatInterval = %v, want 15s", pm.HeartbeatInterval)
	}
	if pm.Metrics != nil {
		t.Errorf("PostgresManager.Metrics should be nil when metricsReg is nil")
	}
	if pm.DB != nil {
		t.Errorf("PostgresManager.DB should be nil (we passed nil)")
	}
	if pm.Store != nil {
		t.Errorf("PostgresManager.Store should be nil (we passed nil)")
	}
}

// TestNew_ClusteredMode_ZeroInterval verifies that a zero heartbeatInterval is
// passed through unchanged (PostgresManager.heartbeatInterval() applies the
// default internally at use time, not at construction).
func TestNew_ClusteredMode_ZeroInterval(t *testing.T) {
	mgr := lease.New("clustered", nil, nil, "pod-z", 0, nil)
	pm, ok := mgr.(*lease.PostgresManager)
	if !ok {
		t.Fatalf("New(\"clustered\") with zero interval returned %T", mgr)
	}
	// Zero is stored as-is; PostgresManager.heartbeatInterval() returns the
	// 10-second default when HeartbeatInterval == 0. Just verify no panic.
	if pm.HeartbeatInterval != 0 {
		t.Errorf("HeartbeatInterval: got %v, want 0 (default applied at use time)", pm.HeartbeatInterval)
	}
}
