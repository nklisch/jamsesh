// Package objectstore — sync.go implements Syncer, the post-receive object-storage
// sync pipeline. It is the durability layer for the cloud-native deploy feature:
// a push does not ack to the git client until SyncPush completes (RPO=0 contract).
package objectstore

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"log/slog"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"time"

	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/storage"
)

// ErrBackpressure is returned by SyncPush when the per-session in-flight
// upload count has reached QueueSize. The caller should respond with 503
// Retry-After to the git client.
//
// Backpressure protects the pod from unbounded memory and file-descriptor
// usage when object-storage writes fall behind the incoming push rate.
var ErrBackpressure = errors.New("objectstore: sync queue full — too many concurrent uploads for this session")

// SyncOutput summarises the work performed by a single SyncPush call.
type SyncOutput struct {
	// ObjectsUploaded is the number of loose git objects uploaded (objects/xx/...).
	ObjectsUploaded int
	// PacksUploaded is the number of pack + idx file pairs uploaded.
	PacksUploaded int
	// RefsChanged is the number of refs whose target SHA changed in the manifest.
	RefsChanged int
	// BytesUploaded is the total bytes written to object storage (objects + packs + manifest).
	BytesUploaded int64
	// Duration is the wall-clock time from the start of SyncPush through manifest save.
	Duration time.Duration
}

// Syncer mirrors the local bare-repo state to object storage after each push.
// It is the sole writer for a session's object-storage namespace while this pod
// holds the lease.
//
// Syncer is safe for concurrent use across different sessions; per-session
// backpressure is tracked in sessionInFlight.
//
// # Long-held lease pattern
//
// SyncPushPath accepts a pre-acquired lease.Handle provided by the caller
// (typically LifecycleManager). The caller holds the lease for the session's
// full lifetime on this pod; SyncPushPath uses the handle's fencing token
// without acquiring or releasing the lease itself.
//
// In single-instance mode pass a handle from lease.NoopManager{}.Acquire —
// it is a zero-cost no-op that provides a zero fencing token. The Emitter
// handles this transparently.
//
// # First-push behaviour
//
// On the first push to a session (no manifest exists yet), SyncPush uploads ALL
// loose objects in the local repo. This is intentionally simple: git repos are
// small during a jamsesh session, so a full object walk is fast (typically
// < 100 ms for a session with a few hundred commits). Subsequent pushes are
// fast because git's content-addressed object store means new objects always
// have new keys — a key we have already uploaded is immutable.
//
// The "track per-session last-synced state" optimisation (mtime cursor, object
// key set) is deferred: the manifest's Packs list and the local filesystem
// serve as sufficient state for pack detection; loose objects are cheap to
// re-attempt via PutIdempotent (idempotent = no-op on repeat).
type Syncer struct {
	// Backend is the object-storage backend. Required.
	Backend Backend
	// Manifests is the manifest store used for linearizable session state. Required.
	Manifests *ManifestStore
	// Storage is the local-FS storage service. Used only for RepoPath(). Required.
	Storage storage.Service
	// Metrics is an optional prometheus registry. When nil, metric emission is
	// skipped. Nil-safe throughout — check before every increment.
	Metrics *metrics.Registry
	// QueueSize is the maximum number of concurrent SyncPush calls allowed for
	// a single session. Calls beyond this limit return ErrBackpressure.
	// Zero is treated as the default (256).
	QueueSize int
	// PerSessionBackpressure enables per-session in-flight counting. Disable
	// only in tests that deliberately flood concurrent pushes to a single session.
	// Defaults to true when the zero value is false — use the explicit setter
	// DisablePerSessionBackpressure in tests if needed.
	PerSessionBackpressure bool

	// sessionInFlight maps sessionID → *int64 (current in-flight count).
	// Values are *int64 managed with atomic operations. Entries are never
	// deleted — sessions are bounded by active sessions on the pod, so the map
	// is O(sessions) in memory, which is fine.
	sessionInFlight sync.Map
}

// queueSize returns the effective queue size with the default applied.
func (s *Syncer) queueSize() int64 {
	if s.QueueSize <= 0 {
		return 256
	}
	return int64(s.QueueSize)
}

// inFlightFor returns the per-session atomic counter, creating it on first use.
func (s *Syncer) inFlightFor(sessionID string) *int64 {
	v, _ := s.sessionInFlight.LoadOrStore(sessionID, new(int64))
	return v.(*int64)
}

// InFlightCount returns the current number of in-flight SyncPush calls for
// the given session. Returns 0 if no uploads have ever been tracked for the
// session (which is indistinguishable from the counter reaching zero after
// draining — both are correct: no pending uploads either way).
//
// This method is the clean API surface consumed by LifecycleManager to drain
// uploads before evicting a session's local cache.
func (s *Syncer) InFlightCount(sessionID string) int64 {
	v, ok := s.sessionInFlight.Load(sessionID)
	if !ok {
		return 0
	}
	return atomic.LoadInt64(v.(*int64))
}

// SyncPushPath enumerates new objects, refs, and pack files in the local bare
// repo at repoPath, uploads them to object storage, and saves an updated
// manifest with conditional-write semantics.
//
// repoPath is the absolute path to the bare git repository on local disk.
// The Emitter derives this via storage.Service.RepoPath(session.OrgID, session.ID).
//
// handle is a pre-acquired lease handle whose fencing token gates all writes.
// The caller (typically LifecycleManager or the Emitter in single-instance
// mode) owns the handle lifetime; SyncPushPath reads the fencing token from
// it but does not acquire or release the handle. If the pod no longer owns the
// session (a newer pod has advanced the manifest's fencing token),
// ManifestStore.Save returns ErrFenced and SyncPushPath propagates it.
//
// Callers should map errors as follows:
//   - ErrFenced       → 503 Service Unavailable (stale lease, abort)
//   - ErrPrecondition → 503 Service Unavailable (concurrent writer)
//   - ErrBackpressure → 503 Service Unavailable + Retry-After
//   - other errors    → 500 Internal Server Error
func (s *Syncer) SyncPushPath(ctx context.Context, sessionID, repoPath string, handle lease.Handle) (SyncOutput, error) {
	start := time.Now()

	// ── Backpressure check ─────────────────────────────────────────────────
	if s.PerSessionBackpressure {
		counter := s.inFlightFor(sessionID)
		cur := atomic.AddInt64(counter, 1)
		defer atomic.AddInt64(counter, -1)

		if cur > s.queueSize() {
			if s.Metrics != nil {
				s.Metrics.ObjectStorageBackpressureTotal.Inc()
				s.Metrics.ObjectStorageUploadsTotal.WithLabelValues("backpressure").Inc()
			}
			return SyncOutput{}, fmt.Errorf("syncpush %s: %w", sessionID, ErrBackpressure)
		}
	}

	fencingToken := handle.FencingToken()

	// ── Load current manifest ──────────────────────────────────────────────
	oldManifest, oldEtag, err := s.Manifests.Load(ctx, sessionID)
	if err != nil {
		s.recordResult("error", start)
		return SyncOutput{}, fmt.Errorf("syncpush %s: load manifest: %w", sessionID, err)
	}

	return s.doSyncAt(ctx, sessionID, repoPath, fencingToken, oldManifest, oldEtag, start)
}

// doSyncAt performs the actual sync against the bare repo at repoPath.
func (s *Syncer) doSyncAt(
	ctx context.Context,
	sessionID, repoPath string,
	fencingToken int64,
	oldManifest Manifest,
	oldEtag string,
	start time.Time,
) (SyncOutput, error) {
	var out SyncOutput

	// ── Step 1: Upload loose objects ───────────────────────────────────────
	objectBytes, objectCount, err := s.uploadLooseObjects(ctx, sessionID, repoPath, fencingToken)
	if err != nil {
		s.recordResult("error", start)
		return SyncOutput{}, fmt.Errorf("syncpush %s: upload loose objects: %w", sessionID, err)
	}
	out.ObjectsUploaded = objectCount
	out.BytesUploaded += objectBytes

	// ── Step 2: Detect and upload new pack files ───────────────────────────
	newPacks, packBytes, packsUploaded, err := s.uploadNewPacks(ctx, sessionID, repoPath, fencingToken, oldManifest.Packs)
	if err != nil {
		s.recordResult("error", start)
		return SyncOutput{}, fmt.Errorf("syncpush %s: upload packs: %w", sessionID, err)
	}
	out.PacksUploaded = packsUploaded
	out.BytesUploaded += packBytes

	// ── Step 3: Read local refs ────────────────────────────────────────────
	newRefs, err := readLocalRefs(repoPath)
	if err != nil {
		s.recordResult("error", start)
		return SyncOutput{}, fmt.Errorf("syncpush %s: read refs: %w", sessionID, err)
	}

	// Count changed refs relative to previous manifest.
	for ref, sha := range newRefs {
		if oldManifest.Refs[ref] != sha {
			out.RefsChanged++
		}
	}
	// Also count deletions.
	for ref := range oldManifest.Refs {
		if _, ok := newRefs[ref]; !ok {
			out.RefsChanged++
		}
	}

	// ── Step 4: Read packed-refs ───────────────────────────────────────────
	packedRefs, err := readPackedRefs(repoPath)
	if err != nil {
		s.recordResult("error", start)
		return SyncOutput{}, fmt.Errorf("syncpush %s: read packed-refs: %w", sessionID, err)
	}

	// ── Step 5: Build and save new manifest ───────────────────────────────
	// Merge old packs with newly uploaded ones. Packs that were in the old
	// manifest but are no longer on local disk will be lazily deleted below.
	newManifest := Manifest{
		Version:      1,
		SessionID:    sessionID,
		Packs:        newPacks,
		Refs:         newRefs,
		PackedRefs:   packedRefs,
		FencingToken: fencingToken,
	}

	newEtag, err := s.Manifests.Save(ctx, newManifest, oldEtag)
	if err != nil {
		if errors.Is(err, ErrFenced) {
			s.recordResult("fenced", start)
			return SyncOutput{}, fmt.Errorf("syncpush %s: %w", sessionID, err)
		}
		if errors.Is(err, ErrPrecondition) {
			s.recordResult("precondition", start)
			return SyncOutput{}, fmt.Errorf("syncpush %s: %w", sessionID, err)
		}
		s.recordResult("error", start)
		return SyncOutput{}, fmt.Errorf("syncpush %s: save manifest: %w", sessionID, err)
	}
	_ = newEtag

	// ── Step 6: Lazy deletion of old packs no longer in new manifest ───────
	oldPackKeys := removedPackKeys(oldManifest.Packs, newPacks)
	if len(oldPackKeys) > 0 {
		go s.lazyDeletePacks(oldPackKeys)
	}

	// ── Metrics on success ─────────────────────────────────────────────────
	out.Duration = time.Since(start)
	if s.Metrics != nil {
		s.Metrics.ObjectStorageUploadsTotal.WithLabelValues("ok").Inc()
		s.Metrics.ObjectStorageUploadBytesTotal.Add(float64(out.BytesUploaded))
		s.Metrics.ObjectStorageUploadDurationSeconds.Observe(out.Duration.Seconds())
	}

	return out, nil
}

// recordResult emits the failure metric for the given result label and
// observes the duration histogram. Used on early-exit error paths.
func (s *Syncer) recordResult(result string, start time.Time) {
	if s.Metrics == nil {
		return
	}
	s.Metrics.ObjectStorageUploadsTotal.WithLabelValues(result).Inc()
	s.Metrics.ObjectStorageUploadDurationSeconds.Observe(time.Since(start).Seconds())
}

// uploadLooseObjects walks objects/xx/* in the bare repo and uploads each
// loose object via PutIdempotent. Returns total bytes uploaded and object count.
//
// Key format: sessions/<sessionID>/objects/<xx>/<rest>
// where <xx> is the two-character fanout directory and <rest> is the remainder
// of the object hash.
//
// ErrAlreadyExists from PutIdempotent is silently ignored: the object is
// content-addressed, so an identical object at the same key is correct behaviour.
func (s *Syncer) uploadLooseObjects(ctx context.Context, sessionID, repoPath string, fencingToken int64) (totalBytes int64, count int, err error) {
	objectsDir := filepath.Join(repoPath, "objects")

	entries, err := os.ReadDir(objectsDir)
	if err != nil {
		if os.IsNotExist(err) {
			return 0, 0, nil
		}
		return 0, 0, fmt.Errorf("read objects dir: %w", err)
	}

	for _, fanoutDir := range entries {
		if !fanoutDir.IsDir() {
			continue
		}
		name := fanoutDir.Name()
		// Skip special subdirectories: info, pack, tmp_*
		if name == "info" || name == "pack" || strings.HasPrefix(name, "tmp_") {
			continue
		}
		// Object fanout dirs are exactly 2 hex characters.
		if len(name) != 2 {
			continue
		}

		fanoutPath := filepath.Join(objectsDir, name)
		objEntries, err := os.ReadDir(fanoutPath)
		if err != nil {
			return totalBytes, count, fmt.Errorf("read fanout dir %s: %w", name, err)
		}

		for _, obj := range objEntries {
			if obj.IsDir() {
				continue
			}
			objPath := filepath.Join(fanoutPath, obj.Name())
			data, err := os.ReadFile(objPath)
			if err != nil {
				// Object may have been packed/deleted since ReadDir; skip it.
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return totalBytes, count, fmt.Errorf("read object %s/%s: %w", name, obj.Name(), err)
			}

			key := "sessions/" + sessionID + "/objects/" + name + "/" + obj.Name()
			putErr := s.Backend.PutIdempotent(ctx, key, data, fencingToken)
			if putErr != nil && !errors.Is(putErr, ErrAlreadyExists) {
				return totalBytes, count, fmt.Errorf("upload object %s/%s: %w", name, obj.Name(), putErr)
			}
			if putErr == nil {
				totalBytes += int64(len(data))
				count++
			}
			// ErrAlreadyExists: object already uploaded with identical content — count as seen but not uploaded.
		}
	}

	return totalBytes, count, nil
}

// uploadNewPacks compares local pack files against the old manifest's pack list
// and uploads any packs not yet in the manifest. Returns the merged pack list
// (old kept packs + new), total bytes uploaded, and count of new packs.
//
// A pack is identified by its SHA (extracted from the filename).
// Key format: sessions/<sessionID>/packs/<sha>.pack and .idx
func (s *Syncer) uploadNewPacks(ctx context.Context, sessionID, repoPath string, fencingToken int64, oldPacks []PackEntry) (allPacks []PackEntry, totalBytes int64, newCount int, err error) {
	packDir := filepath.Join(repoPath, "objects", "pack")

	// Build a set of SHAs already in the manifest.
	manifestSHAs := make(map[string]struct{}, len(oldPacks))
	for _, p := range oldPacks {
		manifestSHAs[p.SHA] = struct{}{}
	}

	// List local .pack files to find SHAs.
	entries, err := os.ReadDir(packDir)
	if err != nil {
		if os.IsNotExist(err) {
			// No pack dir yet — carry over old packs from manifest as-is.
			return append([]PackEntry(nil), oldPacks...), 0, 0, nil
		}
		return nil, 0, 0, fmt.Errorf("read pack dir: %w", err)
	}

	// Map SHA → local filenames for packs we need to upload.
	type localPack struct {
		sha     string
		packFn  string // e.g. "pack-<sha>.pack"
		idxFn   string // e.g. "pack-<sha>.idx"
	}
	localPacks := make(map[string]*localPack)

	for _, e := range entries {
		if e.IsDir() {
			continue
		}
		fn := e.Name()
		if strings.HasPrefix(fn, "pack-") && strings.HasSuffix(fn, ".pack") {
			sha := strings.TrimPrefix(strings.TrimSuffix(fn, ".pack"), "pack-")
			if _, ok := localPacks[sha]; !ok {
				localPacks[sha] = &localPack{sha: sha}
			}
			localPacks[sha].packFn = fn
		} else if strings.HasPrefix(fn, "pack-") && strings.HasSuffix(fn, ".idx") {
			sha := strings.TrimPrefix(strings.TrimSuffix(fn, ".idx"), "pack-")
			if _, ok := localPacks[sha]; !ok {
				localPacks[sha] = &localPack{sha: sha}
			}
			localPacks[sha].idxFn = fn
		}
	}

	// Build final pack list:
	// - Keep all old packs still present locally (or even if gone — manifest
	//   tracks what object storage has; we only remove from manifest if the
	//   pack was replaced by a gc repack, which adds new packs).
	// - Actually, we rebuild from local truth: packs on local disk are the
	//   authoritative set. Old packs not on disk have been gc'd; they get
	//   lazily deleted from object storage.
	var newPacks []PackEntry

	for sha, lp := range localPacks {
		packKey := "sessions/" + sessionID + "/packs/" + sha + ".pack"
		idxKey := "sessions/" + sessionID + "/packs/" + sha + ".idx"

		entry := PackEntry{
			PackKey: packKey,
			IdxKey:  idxKey,
			SHA:     sha,
		}
		newPacks = append(newPacks, entry)

		// Skip upload if already in manifest.
		if _, known := manifestSHAs[sha]; known {
			continue
		}

		// Upload .pack
		if lp.packFn != "" {
			packData, err := os.ReadFile(filepath.Join(packDir, lp.packFn))
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, totalBytes, newCount, fmt.Errorf("read pack file %s: %w", lp.packFn, err)
			}
			putErr := s.Backend.PutIdempotent(ctx, packKey, packData, fencingToken)
			if putErr != nil && !errors.Is(putErr, ErrAlreadyExists) {
				return nil, totalBytes, newCount, fmt.Errorf("upload pack %s: %w", sha, putErr)
			}
			if putErr == nil {
				totalBytes += int64(len(packData))
			}
		}

		// Upload .idx
		if lp.idxFn != "" {
			idxData, err := os.ReadFile(filepath.Join(packDir, lp.idxFn))
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					continue
				}
				return nil, totalBytes, newCount, fmt.Errorf("read idx file %s: %w", lp.idxFn, err)
			}
			putErr := s.Backend.PutIdempotent(ctx, idxKey, idxData, fencingToken)
			if putErr != nil && !errors.Is(putErr, ErrAlreadyExists) {
				return nil, totalBytes, newCount, fmt.Errorf("upload idx %s: %w", sha, putErr)
			}
			if putErr == nil {
				totalBytes += int64(len(idxData))
			}
		}

		newCount++
	}

	return newPacks, totalBytes, newCount, nil
}

// readLocalRefs walks refs/heads and refs/tags in the bare repo and returns a
// map of ref name → SHA. It also reads the HEAD symref target if present.
func readLocalRefs(repoPath string) (map[string]string, error) {
	refs := make(map[string]string)

	refsDir := filepath.Join(repoPath, "refs")
	err := filepath.WalkDir(refsDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.IsDir() {
			return nil
		}

		data, err := os.ReadFile(path)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return fmt.Errorf("read ref %s: %w", path, err)
		}

		sha := strings.TrimSpace(string(data))
		// Relative path from refsDir parent (repoPath) to get the ref name.
		rel, err := filepath.Rel(repoPath, path)
		if err != nil {
			return fmt.Errorf("rel path for ref %s: %w", path, err)
		}
		// Convert OS path separator to forward slash for git ref names.
		refName := filepath.ToSlash(rel)
		refs[refName] = sha

		return nil
	})
	if err != nil && !errors.Is(err, fs.ErrNotExist) {
		return nil, fmt.Errorf("walk refs: %w", err)
	}

	return refs, nil
}

// readPackedRefs reads the packed-refs file from the bare repo. Returns empty
// string if the file does not exist (common for repos without gc).
func readPackedRefs(repoPath string) (string, error) {
	p := filepath.Join(repoPath, "packed-refs")
	data, err := os.ReadFile(p)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", nil
		}
		return "", fmt.Errorf("read packed-refs: %w", err)
	}
	return string(data), nil
}

// removedPackKeys returns the object-storage keys (pack + idx) for packs that
// are in oldPacks but not in newPacks. These are candidates for lazy deletion.
func removedPackKeys(oldPacks, newPacks []PackEntry) []string {
	newSHAs := make(map[string]struct{}, len(newPacks))
	for _, p := range newPacks {
		newSHAs[p.SHA] = struct{}{}
	}

	var keys []string
	for _, p := range oldPacks {
		if _, ok := newSHAs[p.SHA]; !ok {
			keys = append(keys, p.PackKey, p.IdxKey)
		}
	}
	return keys
}

// lazyDeletePacks deletes the given object-storage keys in the background.
// Errors are logged at Warn level and do not block ack. This is a best-effort
// cleanup of pack files that have been superseded by a gc repack.
func (s *Syncer) lazyDeletePacks(keys []string) {
	ctx := context.Background() // detached from push context — don't inherit cancellation
	for _, key := range keys {
		if err := s.Backend.Delete(ctx, key); err != nil {
			slog.Warn("objectstore: lazy pack deletion failed",
				"key", key,
				"err", err,
			)
		}
	}
}

// ---------------------------------------------------------------------------
// packed-refs parsing helper (used in tests; exported for clarity)
// ---------------------------------------------------------------------------

// ParsePackedRefsContent parses the content of a packed-refs file and returns
// a map of ref name → SHA. Lines starting with '#' are skipped. Lines with a
// peeled tag entry ('^') are also skipped.
func ParsePackedRefsContent(content string) map[string]string {
	refs := make(map[string]string)
	sc := bufio.NewScanner(strings.NewReader(content))
	for sc.Scan() {
		line := sc.Text()
		if strings.HasPrefix(line, "#") || strings.HasPrefix(line, "^") {
			continue
		}
		parts := strings.Fields(line)
		if len(parts) < 2 {
			continue
		}
		refs[parts[1]] = parts[0]
	}
	return refs
}
