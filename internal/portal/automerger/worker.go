package automerger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"sync"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	openapi "jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/storage"
)

// Worker is the auto-merger orchestration layer. It subscribes to commit.arrived
// events from events.Log and, for each sync-mode ref, runs the merge + apply
// pipeline in a per-session goroutine backed by a bounded queue.
//
// Construct with all fields populated and call Start(ctx) at portal startup.
// Call Stop(ctx) during shutdown to drain in-flight work.
type Worker struct {
	Store       store.Store
	Storage     storage.Service
	Log         *events.Log
	Applier     *Applier
	PortalHost  string
	IdleTimeout time.Duration // default 30s when zero
	QueueSize   int           // default 256 when zero
	// Metrics is optional; when non-nil, backpressure events increment
	// AutoMergerOutcomes{outcome="backpressure"}.
	Metrics *metrics.Registry

	// internal state — populated by Start
	queues  sync.Map  // sessionID -> chan events.Event
	running sync.Map  // sessionID -> struct{} (sentinel for active goroutine)
	mu      sync.Mutex
	wg      sync.WaitGroup
	unsub   func()
}

// Start subscribes to commit.arrived events, performs a no-op replay scan
// (v1 limitation; see implementation notes), and launches the dispatch
// goroutine. Start returns quickly; background goroutines run until ctx is
// cancelled.
func (w *Worker) Start(ctx context.Context) error {
	if w.IdleTimeout == 0 {
		w.IdleTimeout = 30 * time.Second
	}
	if w.QueueSize == 0 {
		w.QueueSize = 256
	}

	// v1: replay scan skipped intentionally. See implementation notes in the
	// story body for the documented limitation and manual recovery path.
	if err := w.replayScan(ctx); err != nil {
		return err
	}

	ch, unsub := w.Log.Subscribe("commit.arrived")
	w.unsub = unsub

	w.wg.Add(1)
	go w.dispatch(ctx, ch)

	return nil
}

// Stop unsubscribes from the event log and waits for all in-flight per-session
// goroutines to finish (or ctx to expire).
func (w *Worker) Stop(ctx context.Context) error {
	if w.unsub != nil {
		w.unsub()
		w.unsub = nil
	}
	done := make(chan struct{})
	go func() {
		w.wg.Wait()
		close(done)
	}()
	select {
	case <-done:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

// replayScan is a v1 no-op. A session that had commit.arrived events during a
// portal downtime will not be auto-merged until the next push triggers a real
// event. Manual recovery: push a no-op commit on the affected ref, which
// triggers a fresh commit.arrived event and re-activates the worker.
func (w *Worker) replayScan(_ context.Context) error {
	return nil
}

// dispatch is the long-running goroutine that reads from the subscriber channel
// and routes events to per-session queues.
func (w *Worker) dispatch(ctx context.Context, in <-chan events.Event) {
	defer w.wg.Done()
	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-in:
			if !ok {
				// Channel closed by unsubscribe; we are done.
				return
			}
			w.enqueue(ctx, e)
		}
	}
}

// enqueue pushes e onto the per-session queue, creating the queue and starting
// a session worker goroutine if needed. If the queue is full an
// auto-merger.backpressure event is emitted instead.
func (w *Worker) enqueue(ctx context.Context, e events.Event) {
	// LoadOrStore is atomic but Make(chan) is not. Use a mutex to ensure only
	// one goroutine creates the channel per session.
	w.mu.Lock()
	raw, exists := w.queues.Load(e.SessionID)
	if !exists {
		raw = make(chan events.Event, w.QueueSize)
		w.queues.Store(e.SessionID, raw)
	}
	w.mu.Unlock()

	ch := raw.(chan events.Event)
	select {
	case ch <- e:
		w.ensureSessionWorker(ctx, e.SessionID, ch)
	default:
		// Queue full — emit backpressure event.
		w.emitBackpressure(ctx, e)
	}
}

// ensureSessionWorker spawns a per-session worker goroutine if one is not
// already running. The running map acts as the "already-running" guard.
func (w *Worker) ensureSessionWorker(ctx context.Context, sessionID string, ch chan events.Event) {
	if _, loaded := w.running.LoadOrStore(sessionID, struct{}{}); loaded {
		// Already running.
		return
	}
	w.wg.Add(1)
	go w.processSessionQueue(ctx, sessionID, ch)
}

// processSessionQueue drains the per-session event channel until idle timeout
// or ctx cancellation.
func (w *Worker) processSessionQueue(ctx context.Context, sessionID string, ch chan events.Event) {
	defer w.wg.Done()
	defer func() {
		// Mark this worker as no longer running so the next event re-spawns.
		w.running.Delete(sessionID)
	}()

	idle := time.NewTimer(w.IdleTimeout)
	defer idle.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-ch:
			if !ok {
				return
			}
			if !idle.Stop() {
				select {
				case <-idle.C:
				default:
				}
			}
			idle.Reset(w.IdleTimeout)
			w.processEvent(ctx, e)
		case <-idle.C:
			// Idle: clean up queue entry and exit; next event re-spawns.
			w.queues.Delete(sessionID)
			return
		}
	}
}

// processEvent runs the full merge pipeline for one commit.arrived event.
func (w *Worker) processEvent(ctx context.Context, e events.Event) {
	// 1. Decode payload.
	var payload openapi.CommitArrivedPayload
	if err := json.Unmarshal(e.Payload, &payload); err != nil {
		slog.WarnContext(ctx, "automerger worker: bad commit.arrived payload",
			"session_id", e.SessionID,
			"seq", e.Seq,
			"err", err,
		)
		return
	}

	ref := payload.Ref
	sha := payload.Sha

	// 2. Load the session row (need OrgID, DefaultMode, and Status).
	// The event carries OrgID directly, so we can look up without presence rows.
	sess, err := w.Store.GetSession(ctx, e.OrgID, e.SessionID)
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: session lookup failed",
			"session_id", e.SessionID,
			"org_id", e.OrgID,
			"err", err,
		)
		return
	}

	// 3. Look up the ref mode; skip isolated refs.
	mode, err := w.refModeForSession(ctx, e.SessionID, ref, sess.DefaultMode)
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: ref mode lookup failed",
			"session_id", e.SessionID,
			"ref", ref,
			"err", err,
		)
		return
	}
	if mode == "isolated" {
		return
	}

	// 4. Open the bare repo.
	repoPath := w.Storage.RepoPath(sess.OrgID, sess.ID)
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: open repo failed",
			"session_id", e.SessionID,
			"repo_path", repoPath,
			"err", err,
		)
		return
	}

	// 5. Resolve the source commit.
	sourceHash := plumbing.NewHash(sha)
	sourceCommit, err := object.GetCommit(repo.Storer, sourceHash)
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: resolve source commit failed",
			"session_id", e.SessionID,
			"sha", sha,
			"err", err,
		)
		return
	}

	// 6. Resolve the draft tip.
	draftRefName := plumbing.NewBranchReferenceName("jam/" + sess.ID + "/draft")
	draftRef, err := repo.Reference(draftRefName, true)
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: resolve draft ref failed",
			"session_id", e.SessionID,
			"ref", draftRefName.String(),
			"err", err,
		)
		return
	}
	draftCommit, err := object.GetCommit(repo.Storer, draftRef.Hash())
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: resolve draft commit failed",
			"session_id", e.SessionID,
			"sha", draftRef.Hash().String(),
			"err", err,
		)
		return
	}

	// 7. Compute merge base.
	bases, err := sourceCommit.MergeBase(draftCommit)
	if err != nil || len(bases) == 0 {
		slog.WarnContext(ctx, "automerger worker: merge base failed",
			"session_id", e.SessionID,
			"source", sha,
			"draft", draftRef.Hash().String(),
			"err", err,
		)
		return
	}
	ancestor := bases[0]

	// 8. Run the merge engine.
	result, err := Merge(ctx, repo, sourceCommit, draftCommit, ancestor)
	if err != nil {
		slog.WarnContext(ctx, "automerger worker: merge failed",
			"session_id", e.SessionID,
			"err", err,
		)
		return
	}

	// 9. Apply side effects.
	if _, err := w.Applier.Apply(ctx, ApplyInput{
		Repo:         repo,
		Session:      &sess,
		SourceRef:    ref,
		SourceCommit: sourceHash,
		DraftTip:     draftRef.Hash(),
		Ancestor:     ancestor.Hash,
		Result:       result,
		PortalHost:   w.PortalHost,
	}); err != nil {
		slog.WarnContext(ctx, "automerger worker: apply failed",
			"session_id", e.SessionID,
			"err", err,
		)
		return
	}
}

// refModeForSession returns the effective mode for the given (sessionID, ref)
// pair. It checks ref_modes for a per-ref override first; if absent it returns
// the session's defaultMode.
func (w *Worker) refModeForSession(ctx context.Context, sessionID, ref, defaultMode string) (string, error) {
	rm, err := w.Store.GetRefMode(ctx, store.GetRefModeParams{
		SessionID: sessionID,
		Ref:       ref,
	})
	if err == nil {
		return rm.Mode, nil
	}
	if err != store.ErrNotFound {
		return "", fmt.Errorf("get ref mode: %w", err)
	}
	return defaultMode, nil
}

// emitBackpressure emits an auto-merger.backpressure event for the session.
func (w *Worker) emitBackpressure(ctx context.Context, e events.Event) {
	type backpressurePayload struct {
		SessionID string `json:"session_id"`
		DroppedRef string `json:"dropped_ref,omitempty"`
	}
	var p openapi.CommitArrivedPayload
	_ = json.Unmarshal(e.Payload, &p)

	payload, _ := json.Marshal(backpressurePayload{
		SessionID:  e.SessionID,
		DroppedRef: p.Ref,
	})

	if _, err := w.Log.Emit(ctx, e.OrgID, e.SessionID, "auto-merger.backpressure", payload); err != nil {
		slog.WarnContext(ctx, "automerger worker: emit backpressure failed",
			"session_id", e.SessionID,
			"err", err,
		)
	}
	if w.Metrics != nil {
		w.Metrics.AutoMergerOutcomes.WithLabelValues("backpressure").Inc()
	}
}
