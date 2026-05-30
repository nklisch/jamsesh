package automerger

import (
	"context"
	"encoding/json"
	"errors"
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

// workerStore is the minimal store interface consumed by Worker.
type workerStore interface {
	store.SessionStore
	store.RefModeStore
}

// sessionQueue is the per-session state owned by an active draining goroutine.
// Membership in Worker.sessions == "a goroutine is actively draining this queue".
type sessionQueue struct {
	ch chan events.Event
}

// Worker is the auto-merger orchestration layer. It subscribes to commit.arrived
// events from events.Log and, for each sync-mode ref, runs the merge + apply
// pipeline in a per-session goroutine backed by a bounded queue.
//
// Construct with all fields populated and call Start(ctx) at portal startup.
// Call Stop(ctx) during shutdown to drain in-flight work.
type Worker struct {
	Store       workerStore
	Storage     storage.Service
	Log         *events.Log
	Applier     *Applier
	PortalHost  string
	IdleTimeout time.Duration // default 30s when zero
	QueueSize   int           // default 256 when zero
	// Metrics is optional; when non-nil, backpressure events increment
	// AutoMergerOutcomes{outcome="backpressure"}.
	Metrics *metrics.Registry

	// onIdleDecision is a test-only hook invoked under mu inside the idle case,
	// after the len(ch) re-check decision is computed but before acting on it.
	// In production this is nil. Set before Start() is called.
	onIdleDecision func(sessionID string, willExit bool)

	// internal state — populated by Start
	mu       sync.Mutex
	sessions map[string]*sessionQueue // guarded by mu; membership == "worker owns draining"
	wg       sync.WaitGroup
	unsub    func()
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

	w.sessions = make(map[string]*sessionQueue)

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
//
// The queue creation, worker spawn, and event push all happen under w.mu, so
// an idle-exiting goroutine cannot interleave between "queue exists" and "push
// the event" — eliminating the lost-event race that existed with the old
// two-sync.Map design.
func (w *Worker) enqueue(ctx context.Context, e events.Event) {
	w.mu.Lock()
	sq, ok := w.sessions[e.SessionID]
	if !ok {
		sq = &sessionQueue{ch: make(chan events.Event, w.QueueSize)}
		w.sessions[e.SessionID] = sq
		w.wg.Add(1)
		go w.processSessionQueue(ctx, e.SessionID, sq)
	}
	// Push non-blockingly while still holding the lock. Because the channel is
	// buffered and the send is non-blocking, holding the lock is safe — we never
	// block a goroutine while holding mu.
	select {
	case sq.ch <- e:
		w.mu.Unlock()
	default:
		w.mu.Unlock()
		// Queue full — emit backpressure event.
		w.emitBackpressure(ctx, e)
	}
}

// processSessionQueue drains the per-session event channel until idle timeout
// or ctx cancellation.
func (w *Worker) processSessionQueue(ctx context.Context, sessionID string, sq *sessionQueue) {
	defer w.wg.Done()

	idle := time.NewTimer(w.IdleTimeout)
	defer idle.Stop()

	for {
		select {
		case <-ctx.Done():
			// Clean up the sessions entry so Stop + restart works cleanly,
			// but do NOT delete if we're just cancelling — leave the slot for
			// potential future restarts. The entry was created atomically with
			// the goroutine spawn, so it is safe to remove here.
			w.mu.Lock()
			delete(w.sessions, sessionID)
			w.mu.Unlock()
			return
		case e, ok := <-sq.ch:
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
			// Idle: re-check the buffer under mu before deciding to exit.
			// This closes the race window: enqueue may have pushed an event
			// after the idle timer fired but before we could consume it here.
			w.mu.Lock()
			willExit := len(sq.ch) == 0
			if w.onIdleDecision != nil {
				w.onIdleDecision(sessionID, willExit)
			}
			if willExit {
				delete(w.sessions, sessionID)
				w.mu.Unlock()
				return
			}
			// Events arrived during the race window; keep draining.
			w.mu.Unlock()
			idle.Reset(w.IdleTimeout)
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
		if isEmitAfterSideEffect(err) {
			slog.ErrorContext(ctx, "automerger worker: side effect committed but event emit failed — manual recovery: re-push the source ref",
				"session_id", e.SessionID,
				"sha", sha,
			)
			if w.Metrics != nil {
				w.Metrics.AutoMergerOutcomes.WithLabelValues("emit_failed").Inc()
			}
		} else {
			slog.WarnContext(ctx, "automerger worker: apply failed",
				"session_id", e.SessionID,
				"err", err,
			)
		}
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
	if !errors.Is(err, store.ErrNotFound) {
		return "", fmt.Errorf("get ref mode: %w", err)
	}
	return defaultMode, nil
}

// emitBackpressure emits an auto-merger.backpressure event for the session.
func (w *Worker) emitBackpressure(ctx context.Context, e events.Event) {
	type backpressurePayload struct {
		SessionID  string `json:"session_id"`
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
