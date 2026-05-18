// Package objectstore — lifecycle.go implements LifecycleManager, the
// per-pod session lifecycle coordinator for clustered mode.
//
// LifecycleManager owns the per-session state map: it acquires a distributed
// lease and hydrates the local bare repo on first access, then drains in-flight
// uploads and evicts the local cache on release. A background idle-eviction
// loop and an LRU byte-cap enforce memory hygiene without operator intervention.
//
// Single-instance mode never uses LifecycleManager — it is wired only when
// JAMSESH_DEPLOY_MODE is "clustered" (see cmd/portal/main.go, Unit 3).
package objectstore

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"

	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/storage"
)

// LifecycleManager coordinates the per-session lease + local-cache lifecycle
// for clustered-mode portal pods.
//
// On the first request for a session this pod doesn't hold, AcquireForRequest:
//  1. Acquires a distributed lease (errors immediately on ErrAlreadyHeld).
//  2. Hydrates the local bare repo from object storage.
//  3. Stores the session entry for subsequent requests.
//
// On release (explicit, idle timeout, LRU cap, or lease loss), releaseWithReason:
//  1. CAS-flips the releasing flag so concurrent acquires wait.
//  2. Drains in-flight Syncer uploads (bounded 10s).
//  3. Releases the lease handle.
//  4. Removes the local bare repo.
//  5. Removes the entry from the session map.
//
// Start runs the idle-eviction + LRU loop and blocks until ctx is cancelled,
// at which point it releases all remaining sessions with reason "shutdown".
type LifecycleManager struct {
	// Lease is the distributed lease manager. Required.
	Lease lease.Manager

	// Hydrator performs eager download of a session's git objects from object
	// storage into the local bare repo. Required.
	Hydrator *Hydrator

	// Syncer is the upload pipeline. LifecycleManager reads its per-session
	// in-flight counter to drain uploads before evicting. Required.
	Syncer *Syncer

	// Storage is the local-FS storage service. Used for RepoPath during
	// eviction. Required.
	Storage storage.Service

	// OrgIDLookup resolves the org ID for a session ID. Called once per lease
	// lifetime (result cached in sessionEntry). Required.
	OrgIDLookup func(ctx context.Context, sessionID string) (string, error)

	// IdleTimeout is the duration after which an inactive session is evicted.
	// Zero is treated as the default (5 minutes).
	IdleTimeout time.Duration

	// CacheMaxBytes is the maximum cumulative bytes of local bare repos across
	// all active sessions. When exceeded, the least-recently-active session is
	// evicted. Zero means unlimited.
	CacheMaxBytes int64

	// IdleCheckPeriod is how often the idle-eviction + LRU loop runs.
	// Zero is treated as the default (30 seconds).
	IdleCheckPeriod time.Duration

	// Metrics is an optional Prometheus registry. Nil-safe throughout.
	Metrics *metrics.Registry

	// sessions maps sessionID → *sessionEntry. Values are always *sessionEntry.
	sessions sync.Map
}

// sessionEntry is the per-session state held by LifecycleManager.
type sessionEntry struct {
	orgID        string
	handle       lease.Handle
	acquiredAt   time.Time
	lastActiveAt atomic.Pointer[time.Time]
	releasing    atomic.Bool
	// repoSizeBytes is the disk size of the bare repo, refreshed on each
	// eviction-check tick. Used for LRU byte-cap decisions.
	repoSizeBytes atomic.Int64
}

func (e *sessionEntry) touchLastActive() {
	now := time.Now()
	e.lastActiveAt.Store(&now)
}

func (e *sessionEntry) lastActive() time.Time {
	if t := e.lastActiveAt.Load(); t != nil {
		return *t
	}
	return e.acquiredAt
}

// idleTimeout returns the effective idle timeout with the default applied.
func (m *LifecycleManager) idleTimeout() time.Duration {
	if m.IdleTimeout <= 0 {
		return 5 * time.Minute
	}
	return m.IdleTimeout
}

// idleCheckPeriod returns the effective check period with the default applied.
func (m *LifecycleManager) idleCheckPeriod() time.Duration {
	if m.IdleCheckPeriod <= 0 {
		return 30 * time.Second
	}
	return m.IdleCheckPeriod
}

// AcquireForRequest returns the active lease handle for sessionID, acquiring
// and hydrating if this is the first request on this pod.
//
// On success the session's lastActiveAt is updated. The caller MUST NOT
// call handle.Release() directly — lifecycle ownership belongs to
// LifecycleManager. Use Release or releaseWithReason to relinquish a session.
//
// Error cases:
//   - lease.ErrAlreadyHeld: another pod holds the lease; caller should 503.
//   - hydration error: lease released, nothing stored; caller should 503.
//   - any context error: propagated.
func (m *LifecycleManager) AcquireForRequest(ctx context.Context, sessionID string) (lease.Handle, error) {
	for {
		if v, ok := m.sessions.Load(sessionID); ok {
			entry := v.(*sessionEntry)
			if !entry.releasing.Load() {
				entry.touchLastActive()
				return entry.handle, nil
			}
			// Entry is in the middle of being released. Wait briefly and retry.
			select {
			case <-ctx.Done():
				return nil, ctx.Err()
			case <-time.After(10 * time.Millisecond):
			}
			continue
		}

		// No entry exists — acquire from scratch.
		return m.acquireNew(ctx, sessionID)
	}
}

// acquireNew performs the full acquire-hydrate sequence for a session that is
// not (or no longer) in the sessions map.
func (m *LifecycleManager) acquireNew(ctx context.Context, sessionID string) (lease.Handle, error) {
	orgID, err := m.OrgIDLookup(ctx, sessionID)
	if err != nil {
		return nil, fmt.Errorf("lifecycle: org lookup for %s: %w", sessionID, err)
	}

	handle, err := m.Lease.Acquire(ctx, sessionID)
	if err != nil {
		if errors.Is(err, lease.ErrAlreadyHeld) {
			return nil, fmt.Errorf("lifecycle: session %s: %w", sessionID, err)
		}
		return nil, fmt.Errorf("lifecycle: acquire lease for %s: %w", sessionID, err)
	}

	if _, hydrateErr := m.Hydrator.Hydrate(ctx, orgID, sessionID); hydrateErr != nil {
		_ = handle.Release()
		return nil, fmt.Errorf("lifecycle: hydrate %s: %w", sessionID, hydrateErr)
	}

	now := time.Now()
	entry := &sessionEntry{
		orgID:      orgID,
		handle:     handle,
		acquiredAt: now,
	}
	entry.lastActiveAt.Store(&now)

	// Store atomically. If another goroutine raced us and already inserted an
	// entry, prefer the winner and release our duplicate handle.
	actual, loaded := m.sessions.LoadOrStore(sessionID, entry)
	if loaded {
		_ = handle.Release()
		winner := actual.(*sessionEntry)
		winner.touchLastActive()
		return winner.handle, nil
	}

	// We won the race — wire up the lost-lease watcher.
	go m.watchLost(handle, sessionID)

	if m.Metrics != nil && m.Metrics.LifecycleActiveSessions != nil {
		m.Metrics.LifecycleActiveSessions.Inc()
	}

	slog.Info("lifecycle: session acquired",
		"session_id", sessionID,
		"org_id", orgID,
		"fencing_token", handle.FencingToken(),
	)

	return handle, nil
}

// watchLost blocks until handle.Lost() fires, then triggers an automatic
// release of the session with reason "lost".
func (m *LifecycleManager) watchLost(handle lease.Handle, sessionID string) {
	<-handle.Lost()
	slog.Warn("lifecycle: lease lost, triggering release",
		"session_id", sessionID,
	)
	if m.Metrics != nil && m.Metrics.LeaseLostTotal != nil {
		m.Metrics.LeaseLostTotal.Inc()
	}
	_ = m.releaseWithReason(context.Background(), sessionID, "lost")
}

// releaseWithReason performs the full release sequence:
//  1. CAS releasing flag false → true (idempotent guard).
//  2. Drain in-flight Syncer uploads (bounded 10s).
//  3. Release the lease handle.
//  4. Remove the local bare repo.
//  5. Delete the session entry.
//
// reason is recorded in the eviction metric label and log line.
func (m *LifecycleManager) releaseWithReason(ctx context.Context, sessionID, reason string) error {
	v, ok := m.sessions.Load(sessionID)
	if !ok {
		return nil // already gone
	}
	entry := v.(*sessionEntry)

	if !entry.releasing.CompareAndSwap(false, true) {
		return nil // another goroutine is handling the release
	}

	// Drain in-flight uploads for this session. Bounded at 10s.
	if m.Syncer != nil {
		drainCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
		defer cancel()
		const pollInterval = 50 * time.Millisecond
		for {
			if m.Syncer.InFlightCount(sessionID) == 0 {
				break
			}
			select {
			case <-drainCtx.Done():
				slog.Warn("lifecycle: timed out draining in-flight uploads before eviction",
					"session_id", sessionID,
					"reason", reason,
				)
				break
			case <-time.After(pollInterval):
			}
			if drainCtx.Err() != nil {
				break
			}
		}
	}

	// Release the distributed lease.
	if releaseErr := entry.handle.Release(); releaseErr != nil {
		slog.Warn("lifecycle: handle.Release error",
			"session_id", sessionID,
			"reason", reason,
			"err", releaseErr,
		)
		// Continue — we still want to evict the local cache.
	}

	// Remove the local bare repo. Local disk is cache only; object storage holds truth.
	repoPath := m.Storage.RepoPath(entry.orgID, sessionID)
	if removeErr := os.RemoveAll(repoPath); removeErr != nil {
		slog.Warn("lifecycle: os.RemoveAll failed",
			"session_id", sessionID,
			"repo_path", repoPath,
			"err", removeErr,
		)
		// Non-fatal — stale disk will be cleaned on next eviction or pod restart.
	}

	// Remove the session entry.
	m.sessions.Delete(sessionID)

	// Emit metrics.
	if m.Metrics != nil {
		if m.Metrics.LifecycleActiveSessions != nil {
			m.Metrics.LifecycleActiveSessions.Dec()
		}
		if m.Metrics.LifecycleEvictionsTotal != nil {
			m.Metrics.LifecycleEvictionsTotal.WithLabelValues(reason).Inc()
		}
	}

	slog.Info("lifecycle: session released",
		"session_id", sessionID,
		"org_id", entry.orgID,
		"reason", reason,
		"held_for", time.Since(entry.acquiredAt).Round(time.Millisecond),
	)

	return nil
}

// Start runs the idle-eviction + LRU loop. It blocks until ctx is cancelled,
// then drains all remaining sessions (reason "shutdown") before returning.
// Callers should run Start in a dedicated goroutine.
func (m *LifecycleManager) Start(ctx context.Context) error {
	ticker := time.NewTicker(m.idleCheckPeriod())
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			m.shutdownAll(ctx)
			return ctx.Err()
		case <-ticker.C:
			m.evictIdleAndOversize(context.Background())
		}
	}
}

// evictIdleAndOversize performs one pass of idle eviction followed by LRU
// eviction if the cumulative cache size exceeds CacheMaxBytes. Called on each
// idle-check tick and during tests.
func (m *LifecycleManager) evictIdleAndOversize(ctx context.Context) {
	idleTimeout := m.idleTimeout()
	now := time.Now()

	type candidate struct {
		sessionID     string
		lastActive    time.Time
		repoSizeBytes int64
	}

	var active []candidate

	// Refresh per-session repo sizes and collect idle-eviction candidates.
	m.sessions.Range(func(k, v any) bool {
		sessionID := k.(string)
		entry := v.(*sessionEntry)

		if entry.releasing.Load() {
			return true
		}

		// Refresh repo size for LRU decisions.
		repoPath := m.Storage.RepoPath(entry.orgID, sessionID)
		sz := dirSize(repoPath)
		entry.repoSizeBytes.Store(sz)

		la := entry.lastActive()
		if now.Sub(la) > idleTimeout {
			slog.Info("lifecycle: idle eviction",
				"session_id", sessionID,
				"idle_for", now.Sub(la).Round(time.Second),
			)
			_ = m.releaseWithReason(ctx, sessionID, "idle")
			return true
		}

		active = append(active, candidate{
			sessionID:     sessionID,
			lastActive:    la,
			repoSizeBytes: sz,
		})
		return true
	})

	// LRU eviction: while cumulative size exceeds the cap, evict the least-
	// recently-active session.
	if m.CacheMaxBytes <= 0 || len(active) == 0 {
		return
	}

	for {
		var totalBytes int64
		for _, c := range active {
			totalBytes += c.repoSizeBytes
		}
		if totalBytes <= m.CacheMaxBytes {
			break
		}

		// Find the least-recently-active session (oldest lastActive time).
		lruIdx := 0
		for i, c := range active {
			if c.lastActive.Before(active[lruIdx].lastActive) {
				lruIdx = i
			}
		}

		victim := active[lruIdx]
		slog.Info("lifecycle: LRU eviction",
			"session_id", victim.sessionID,
			"cache_total_bytes", totalBytes,
			"cache_max_bytes", m.CacheMaxBytes,
		)
		_ = m.releaseWithReason(ctx, victim.sessionID, "lru")

		// Remove from active slice and continue.
		active = append(active[:lruIdx], active[lruIdx+1:]...)
		if len(active) == 0 {
			break
		}
	}
}

// shutdownAll releases all active sessions with reason "shutdown". Each
// release runs in its own goroutine bounded to 30 seconds. The call blocks
// until all goroutines complete.
func (m *LifecycleManager) shutdownAll(ctx context.Context) {
	var wg sync.WaitGroup

	m.sessions.Range(func(k, _ any) bool {
		sessionID := k.(string)
		wg.Add(1)
		go func() {
			defer wg.Done()
			releaseCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
			defer cancel()
			if err := m.releaseWithReason(releaseCtx, sessionID, "shutdown"); err != nil {
				slog.Warn("lifecycle: shutdown release error",
					"session_id", sessionID,
					"err", err,
				)
			}
		}()
		return true
	})

	wg.Wait()
}

// dirSize returns the cumulative size of all regular files under path.
// Returns 0 if path does not exist or cannot be walked.
func dirSize(path string) int64 {
	var total int64
	_ = filepath.Walk(path, func(_ string, info os.FileInfo, err error) error {
		if err != nil {
			return nil // skip unreadable entries
		}
		if !info.IsDir() {
			total += info.Size()
		}
		return nil
	})
	return total
}
