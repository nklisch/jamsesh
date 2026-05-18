// Package objectstore — hydrate.go implements Hydrator, the pure download +
// write logic for session hydration. A Hydrator downloads a session's manifest,
// pack files, loose objects, and refs from object storage and writes them to a
// local-FS bare repository. It is the reverse of Syncer: where Syncer pushes
// from local disk to object storage, Hydrator pulls from object storage to
// local disk.
//
// Hydrator has no awareness of leases, idle timers, or eviction — those
// concerns belong to LifecycleManager (Unit 2). Hydrator is a pure value-type
// computation: given an object-storage Backend, it fills a local bare repo and
// returns a summary of what was downloaded.
package objectstore

import (
	"context"
	"fmt"
	"log/slog"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sync/errgroup"

	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/storage"
)

// Hydrator downloads a session's git objects from object storage into a local
// bare repository. It is safe for concurrent use across different sessions.
//
// Hydrator is intentionally stateless — all per-call state lives in Hydrate's
// stack frame. Multiple calls with the same (orgID, sessionID) pair are safe
// (idempotent: existing objects are overwritten atomically).
type Hydrator struct {
	// Backend is the object-storage backend. Required.
	Backend Backend

	// Manifests is the manifest store used to look up the session's pack + ref
	// list. Required.
	Manifests *ManifestStore

	// Storage is the local-FS storage service. Used for RepoPath and
	// CreateRepo. Required.
	Storage storage.Service

	// Metrics is an optional prometheus registry. When nil, metric emission is
	// skipped. Nil-safe throughout.
	Metrics *metrics.Registry

	// Workers is the maximum number of concurrent download goroutines.
	// Zero is treated as the default (8).
	Workers int
}

// HydrationOutput summarises the work performed by a single Hydrate call.
type HydrationOutput struct {
	// ObjectsDownloaded is the number of loose git objects written to the local
	// bare repo (objects/xx/yyyy...).
	ObjectsDownloaded int

	// PacksDownloaded is the number of pack + idx file pairs written to the
	// local bare repo (objects/pack/).
	PacksDownloaded int

	// BytesDownloaded is the total bytes written to disk from object storage
	// (loose objects + pack files + idx files; manifest not counted).
	BytesDownloaded int64

	// Duration is the wall-clock time from the start of Hydrate through fsck
	// completion.
	Duration time.Duration

	// FsckOK reports whether `git fsck --no-dangling` exited zero on the
	// hydrated repo. A false value is logged at Warn level but does not cause
	// Hydrate to return an error — the caller decides whether to serve the repo.
	FsckOK bool
}

// workers returns the effective download worker count with the default applied.
func (h *Hydrator) workers() int {
	if h.Workers <= 0 {
		return 8
	}
	return h.Workers
}

// Hydrate downloads the session's git objects from object storage into the
// local bare repository and verifies integrity via git fsck.
//
// If the session has no manifest (fresh session), Hydrate calls
// Storage.CreateRepo to initialise an empty bare repo and returns immediately
// with zero counts and FsckOK=true (vacuously OK — an empty repo is always
// valid).
//
// If Storage.CreateRepo returns an "already exists" error, Hydrate treats it
// as success (the caller may have already initialised the repo).
//
// Hydrate is idempotent for existing sessions: downloaded objects are written
// atomically (tmp + rename), so a retry after a partial failure is safe.
func (h *Hydrator) Hydrate(ctx context.Context, orgID, sessionID string) (HydrationOutput, error) {
	start := time.Now()

	// ── Step 1: Load manifest ─────────────────────────────────────────────────
	manifest, etag, err := h.Manifests.Load(ctx, sessionID)
	if err != nil {
		return HydrationOutput{}, fmt.Errorf("hydrate %s: load manifest: %w", sessionID, err)
	}

	// Fresh session — no manifest in object storage yet.
	if etag == "" {
		if createErr := h.Storage.CreateRepo(ctx, orgID, sessionID); createErr != nil {
			// "already exists" is acceptable — the repo was initialised by a
			// prior call or a concurrent goroutine.
			if !isAlreadyExistsErr(createErr) {
				return HydrationOutput{}, fmt.Errorf("hydrate %s: create repo (fresh session): %w", sessionID, createErr)
			}
		}
		out := HydrationOutput{FsckOK: true}
		out.Duration = time.Since(start)
		h.recordMetrics(ctx, "fresh", out)
		return out, nil
	}

	// ── Step 2: Ensure local bare repo exists ─────────────────────────────────
	repoPath := h.Storage.RepoPath(orgID, sessionID)
	exists, err := h.Storage.RepoExists(orgID, sessionID)
	if err != nil {
		return HydrationOutput{}, fmt.Errorf("hydrate %s: check repo exists: %w", sessionID, err)
	}
	if !exists {
		if createErr := h.Storage.CreateRepo(ctx, orgID, sessionID); createErr != nil {
			if !isAlreadyExistsErr(createErr) {
				return HydrationOutput{}, fmt.Errorf("hydrate %s: create repo: %w", sessionID, createErr)
			}
		}
	}

	var out HydrationOutput

	// ── Step 3: Parallel download packs + idx ────────────────────────────────
	packsDownloaded, packBytes, err := h.downloadPacks(ctx, manifest.Packs, repoPath)
	if err != nil {
		return HydrationOutput{}, fmt.Errorf("hydrate %s: download packs: %w", sessionID, err)
	}
	out.PacksDownloaded = packsDownloaded
	out.BytesDownloaded += packBytes

	// ── Step 4: Parallel download loose objects ───────────────────────────────
	//
	// Syncer writes loose objects at: sessions/<id>/objects/<xx>/<rest>
	// Packs are written at:          sessions/<id>/packs/<sha>.pack (and .idx)
	//
	// We enumerate under sessions/<id>/objects/ — this prefix includes both
	// fanout directories (objects/<xx>/<rest>) and the pack subdirectory
	// (objects/pack/...) IF Syncer wrote packs under objects/pack/. However,
	// Syncer actually writes packs under sessions/<id>/packs/ (separate from
	// objects/), so the objects/ listing contains only loose objects. We still
	// filter any "pack/" prefix entries as a safety net against future changes.
	objectsPrefix := "sessions/" + sessionID + "/objects/"
	objectsDownloaded, objectBytes, err := h.downloadLooseObjects(ctx, objectsPrefix, repoPath)
	if err != nil {
		return HydrationOutput{}, fmt.Errorf("hydrate %s: download loose objects: %w", sessionID, err)
	}
	out.ObjectsDownloaded = objectsDownloaded
	out.BytesDownloaded += objectBytes

	// ── Step 5: Write refs ───────────────────────────────────────────────────
	for refName, sha := range manifest.Refs {
		refPath := filepath.Join(repoPath, filepath.FromSlash(refName))
		if err := writeAtomic(refPath, []byte(sha+"\n")); err != nil {
			return HydrationOutput{}, fmt.Errorf("hydrate %s: write ref %s: %w", sessionID, refName, err)
		}
	}

	// ── Step 6: Write packed-refs ────────────────────────────────────────────
	if manifest.PackedRefs != "" {
		packedRefsPath := filepath.Join(repoPath, "packed-refs")
		if err := writeAtomic(packedRefsPath, []byte(manifest.PackedRefs)); err != nil {
			return HydrationOutput{}, fmt.Errorf("hydrate %s: write packed-refs: %w", sessionID, err)
		}
	}

	// ── Step 7: git fsck ──────────────────────────────────────────────────────
	out.FsckOK = h.runFsck(ctx, sessionID, repoPath)

	// ── Step 8: Metrics ───────────────────────────────────────────────────────
	out.Duration = time.Since(start)
	if out.FsckOK {
		h.recordMetrics(ctx, "ok", out)
	} else {
		h.recordMetrics(ctx, "error", out)
	}

	return out, nil
}

// downloadPacks downloads all pack + idx file pairs listed in the manifest into
// <repoPath>/objects/pack/ using bounded parallelism. Returns the number of
// pack pairs downloaded and total bytes.
func (h *Hydrator) downloadPacks(ctx context.Context, packs []PackEntry, repoPath string) (count int, totalBytes int64, err error) {
	if len(packs) == 0 {
		return 0, 0, nil
	}

	packDir := filepath.Join(repoPath, "objects", "pack")
	if mkErr := os.MkdirAll(packDir, 0o755); mkErr != nil {
		return 0, 0, fmt.Errorf("mkdir pack dir: %w", mkErr)
	}

	type result struct {
		bytes int
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(h.workers())

	results := make(chan result, len(packs)*2) // 2 files per pack (pack + idx)

	for _, pack := range packs {
		pack := pack // capture loop variable
		sha := pack.SHA

		// Download .pack file
		g.Go(func() error {
			data, _, _, getErr := h.Backend.Get(gctx, pack.PackKey)
			if getErr != nil {
				return fmt.Errorf("get pack %s: %w", sha, getErr)
			}
			packPath := filepath.Join(packDir, "pack-"+sha+".pack")
			if writeErr := writeAtomic(packPath, data); writeErr != nil {
				return fmt.Errorf("write pack %s: %w", sha, writeErr)
			}
			results <- result{bytes: len(data)}
			return nil
		})

		// Download .idx file
		g.Go(func() error {
			data, _, _, getErr := h.Backend.Get(gctx, pack.IdxKey)
			if getErr != nil {
				return fmt.Errorf("get idx %s: %w", sha, getErr)
			}
			idxPath := filepath.Join(packDir, "pack-"+sha+".idx")
			if writeErr := writeAtomic(idxPath, data); writeErr != nil {
				return fmt.Errorf("write idx %s: %w", sha, writeErr)
			}
			results <- result{bytes: len(data)}
			return nil
		})
	}

	// Wait for all goroutines, then close the results channel.
	waitErr := g.Wait()
	close(results)

	for r := range results {
		totalBytes += int64(r.bytes)
	}

	if waitErr != nil {
		return 0, totalBytes, waitErr
	}

	return len(packs), totalBytes, nil
}

// downloadLooseObjects enumerates all loose object keys under objectsPrefix via
// Backend.List, then downloads each in parallel into the local bare repo's
// objects/ directory. Returns the number of objects downloaded and total bytes.
//
// Keys under objectsPrefix/<something>/pack/ are skipped — those are pack files
// already handled by downloadPacks.
func (h *Hydrator) downloadLooseObjects(ctx context.Context, objectsPrefix, repoPath string) (count int, totalBytes int64, err error) {
	// Collect all loose object keys first (List may not be safe to call
	// concurrently with goroutine dispatch if the backend holds a mutex during
	// iteration).
	var keys []string
	listErr := h.Backend.List(ctx, objectsPrefix, func(key string) error {
		// key is the full logical key, e.g. "sessions/<id>/objects/ab/cdef..."
		// Strip the objectsPrefix to get the relative portion.
		rel := strings.TrimPrefix(key, objectsPrefix)
		// Skip pack subdirectory entries — packs are at sessions/<id>/packs/ not
		// objects/, but guard defensively.
		if strings.HasPrefix(rel, "pack/") || strings.HasPrefix(rel, "pack\\") {
			return nil
		}
		// Only process fanout entries: relative path must be xx/rest (2-char hex prefix).
		if len(rel) < 4 || rel[2] != '/' {
			return nil
		}
		keys = append(keys, key)
		return nil
	})
	if listErr != nil {
		return 0, 0, fmt.Errorf("list loose objects: %w", listErr)
	}

	if len(keys) == 0 {
		return 0, 0, nil
	}

	type result struct {
		bytes int
	}

	g, gctx := errgroup.WithContext(ctx)
	g.SetLimit(h.workers())

	results := make(chan result, len(keys))

	for _, key := range keys {
		key := key // capture loop variable

		g.Go(func() error {
			data, _, _, getErr := h.Backend.Get(gctx, key)
			if getErr != nil {
				return fmt.Errorf("get object %s: %w", key, getErr)
			}

			// Reconstruct local path: objects/<xx>/<rest>
			// key = "sessions/<id>/objects/<xx>/<rest>"
			rel := strings.TrimPrefix(key, objectsPrefix)
			// rel = "<xx>/<rest>"
			objPath := filepath.Join(repoPath, "objects", filepath.FromSlash(rel))
			if writeErr := writeAtomic(objPath, data); writeErr != nil {
				return fmt.Errorf("write object %s: %w", rel, writeErr)
			}
			results <- result{bytes: len(data)}
			return nil
		})
	}

	waitErr := g.Wait()
	close(results)

	for r := range results {
		totalBytes += int64(r.bytes)
		count++
	}

	if waitErr != nil {
		return count, totalBytes, waitErr
	}

	return count, totalBytes, nil
}

// runFsck runs `git fsck --no-dangling` on the local bare repo and returns
// true if it exits zero. A non-zero exit is logged at Warn level but is not
// returned as an error — the caller decides how to handle an unhealthy repo.
func (h *Hydrator) runFsck(ctx context.Context, sessionID, repoPath string) bool {
	cmd := exec.CommandContext(ctx, "git", "-C", repoPath, "fsck", "--no-dangling")
	out, err := cmd.CombinedOutput()
	if err != nil {
		slog.Warn("objectstore: git fsck failed after hydration",
			"session_id", sessionID,
			"repo_path", repoPath,
			"stderr", string(out),
			"err", err,
		)
		return false
	}
	return true
}

// recordMetrics emits hydration metrics to the registry when non-nil.
// result is one of: "ok", "fresh", "error".
func (h *Hydrator) recordMetrics(_ context.Context, result string, out HydrationOutput) {
	if h.Metrics == nil {
		return
	}
	if h.Metrics.HydrationsTotal != nil {
		h.Metrics.HydrationsTotal.WithLabelValues(result).Inc()
	}
	if h.Metrics.HydrationDurationSeconds != nil {
		h.Metrics.HydrationDurationSeconds.Observe(out.Duration.Seconds())
	}
	if h.Metrics.HydrationBytesTotal != nil {
		h.Metrics.HydrationBytesTotal.Add(float64(out.BytesDownloaded))
	}
}

// writeAtomic writes data to path atomically by writing to a ".tmp" sibling
// then renaming. The destination directory is created if it does not exist.
// Any stale ".tmp" file is removed if the write fails (cleanup is best-effort;
// the caller should not rely on tmp files being absent after an error).
func writeAtomic(path string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("writeAtomic mkdir: %w", err)
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("writeAtomic write: %w", err)
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp) // best-effort cleanup
		return fmt.Errorf("writeAtomic rename: %w", err)
	}
	return nil
}

// isAlreadyExistsErr reports whether err is a "repo already exists" error from
// storage.Service.CreateRepo. The service returns a formatted error string (not
// a sentinel), so we match on the substring.
func isAlreadyExistsErr(err error) bool {
	return err != nil && strings.Contains(err.Error(), "already exists")
}
