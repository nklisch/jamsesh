package portalcluster

import (
	"context"
	"database/sql"
	"os/exec"
	"syscall"
	"testing"
	"time"

	_ "github.com/lib/pq"
)

// GracefulDrain sends SIGTERM to the indicated pod and waits for clean
// shutdown. Returns once the pod's container is in "exited" status, or
// t.Fatal on timeout.
//
// podIndex must be a valid index into c.Pods; t.Fatal is called otherwise.
func (c *Cluster) GracefulDrain(ctx context.Context, t *testing.T, podIndex int, timeout time.Duration) {
	t.Helper()
	if podIndex < 0 || podIndex >= len(c.Pods) {
		t.Fatalf("GracefulDrain: podIndex %d out of range (cluster has %d pods)", podIndex, len(c.Pods))
	}

	p := c.Pods[podIndex]
	if err := p.SendSignal(ctx, syscall.SIGTERM); err != nil {
		t.Fatalf("GracefulDrain pod %d: send SIGTERM: %v", podIndex, err)
	}

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		st, err := p.State(ctx)
		if err == nil && st.Status == "exited" {
			return
		}
		time.Sleep(200 * time.Millisecond)
	}
	t.Fatalf("GracefulDrain pod %d: did not exit within %v", podIndex, timeout)
}

// Kill terminates the indicated pod abruptly by sending SIGKILL via the Docker
// daemon. This is suitable for simulating a hard pod crash in lease-fencing
// scenarios.
//
// The implementation uses `docker kill --signal SIGKILL <container-name>` which
// is equivalent to a hard kill without going through the in-container process.
// This is simpler and more reliable than Pumba for SIGKILL-only scenarios;
// Pumba adds value for network-level chaos (packet loss, latency) which is
// outside the scope of this fixture.
//
// podIndex must be a valid index into c.Pods; t.Fatal is called otherwise.
func (c *Cluster) Kill(ctx context.Context, t *testing.T, podIndex int) {
	t.Helper()
	if podIndex < 0 || podIndex >= len(c.Pods) {
		t.Fatalf("Kill: podIndex %d out of range (cluster has %d pods)", podIndex, len(c.Pods))
	}

	p := c.Pods[podIndex]
	name := p.ContainerName(ctx)
	if name == "" {
		t.Fatalf("Kill pod %d: could not get container name — cannot kill", podIndex)
	}

	out, err := exec.CommandContext(ctx, "docker", "kill", "--signal", "SIGKILL", name).CombinedOutput()
	if err != nil {
		t.Fatalf("Kill pod %d: docker kill %q: %v\n%s", podIndex, name, err, out)
	}
}

// LeaseHolder queries Postgres pg_locks to find which pod currently holds the
// advisory lock for the given sessionID. Returns the pod's index in c.Pods, or
// -1 if no lock is held by any known pod.
//
// Implementation: the portal acquires a 32-bit advisory lock keyed on
// hashtext(sessionID)::int4 (matching the lease-fencing implementation
// documented in docs/ARCHITECTURE.md). The lock holder's pg_stat_activity row
// carries the client_addr of the pod's connection, which is cross-referenced
// against each pod's container IP to determine which pod holds the lock.
//
// KNOWN RISK — hashtext portability:
//
// PostgreSQL's hashtext() function is not guaranteed stable across major
// versions. This fixture targets postgres:16-alpine (the e2e Postgres image).
// If the portal's lease-fencing implementation uses a different hash function
// or a different bit-width, LeaseHolder will return -1 even when a lock is
// held. If downstream lease-fencing tests find LeaseHolder always returns -1
// despite a lease being acquired, the recommended mitigation is to add a
// portal-side /test/lease-debug endpoint (behind a build tag) that exposes the
// exact advisory-lock key — that is a follow-on story and out of scope here.
func (c *Cluster) LeaseHolder(ctx context.Context, t *testing.T, sessionID string) int {
	t.Helper()

	db, err := sql.Open("postgres", c.postgres.DSN)
	if err != nil {
		t.Fatalf("LeaseHolder: open admin connection: %v", err)
	}
	defer db.Close()

	// Find all connections that hold an advisory lock whose objid matches
	// hashtext(sessionID)::oid. Join pg_stat_activity to get the client IP.
	// pg_locks.objid is type oid (unsigned 32-bit). The portal's own lease
	// code (internal/portal/lease/postgres_test.go) consistently casts to
	// ::oid; matching that convention avoids divergence on negative hashtext
	// values where ::bigint sign-extends and ::oid wraps differently.
	const query = `
		SELECT a.client_addr::text
		FROM pg_locks l
		JOIN pg_stat_activity a ON l.pid = a.pid
		WHERE l.locktype = 'advisory'
		  AND l.objid = hashtext($1)::oid
		  AND l.granted = true
	`
	rows, err := db.QueryContext(ctx, query, sessionID)
	if err != nil {
		t.Fatalf("LeaseHolder: query pg_locks: %v", err)
	}
	defer rows.Close()

	var holderAddrs []string
	for rows.Next() {
		var addr sql.NullString
		if err := rows.Scan(&addr); err != nil {
			t.Fatalf("LeaseHolder: scan row: %v", err)
		}
		if addr.Valid && addr.String != "" {
			holderAddrs = append(holderAddrs, addr.String)
		}
	}
	if err := rows.Err(); err != nil {
		t.Fatalf("LeaseHolder: iterate rows: %v", err)
	}

	if len(holderAddrs) == 0 {
		return -1
	}

	// Cross-reference holder IPs against pod container IPs.
	for i, p := range c.Pods {
		podIP, err := p.ContainerIP(ctx)
		if err != nil {
			// Log but continue — one unreachable pod shouldn't block others.
			t.Logf("LeaseHolder: get IP for pod %d: %v", i, err)
			continue
		}
		for _, holderAddr := range holderAddrs {
			if holderAddr == podIP {
				return i
			}
		}
	}

	// Lock is held but none of the cluster's pods match the holder IP.
	// This can happen transiently if a pod is being replaced, or if
	// hashtext() returns a key that collides with an unrelated advisory lock.
	t.Logf("LeaseHolder: lock held by %v but no cluster pod matches — returning -1", holderAddrs)
	return -1
}

// WaitForLeaseMigration polls LeaseHolder until the holder differs from
// fromPod, or until timeout elapses. Returns the new holder index (>= 0) on
// success, or -1 on timeout.
//
// A return of -1 from this method means the lease did not migrate within the
// timeout — the caller should treat this as a test failure or an indication
// that the lease-migration interval needs tuning via PortalExtraEnv.
func (c *Cluster) WaitForLeaseMigration(ctx context.Context, t *testing.T, sessionID string, fromPod int, timeout time.Duration) int {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		holder := c.LeaseHolder(ctx, t, sessionID)
		if holder >= 0 && holder != fromPod {
			return holder
		}
		time.Sleep(200 * time.Millisecond)
	}
	return -1
}
