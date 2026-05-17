package automerger

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/oklog/ulid/v2"

	openapi "jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/prereceive"
)

// Applier is the side-effecting counterpart to the pure merge engine. It
// takes a [MergeResult] and either creates a merge commit + advances the draft
// ref, or inserts a conflict_events row and emits the appropriate events.
//
// Construct once per worker lifetime via [NewApplier] and share it.
type Applier struct {
	Store   store.Store
	Log     *events.Log
	// Metrics is optional; when non-nil, auto-merger outcomes increment
	// AutoMergerOutcomes with outcome labels "succeeded", "conflict", or "backpressure".
	Metrics *metrics.Registry
}

// NewApplier returns an Applier backed by the given store and event log.
func NewApplier(s store.Store, log *events.Log) *Applier {
	return &Applier{Store: s, Log: log}
}

// ApplyInput contains all the inputs required for Apply.
type ApplyInput struct {
	Repo         *gogit.Repository
	Session      *store.Session
	SourceRef    string        // e.g. "refs/heads/jam/<sess>/<user>/<branch>"
	SourceCommit plumbing.Hash // the incoming commit being merged
	DraftTip     plumbing.Hash // current tip of jam/<sess>/draft
	Ancestor     plumbing.Hash // common merge-base
	Result       MergeResult
	PortalHost   string // e.g. "jamsesh.example.com"
}

// ApplyOutput is the result of Apply.
type ApplyOutput struct {
	// MergeCommitSHA is set on clean merge or safe-auto-resolve.
	MergeCommitSHA string
	// ConflictEvent is set on hard conflict.
	ConflictEvent *store.ConflictEvent
}

// Apply executes the side effects for a MergeResult.
//
//   - CleanMerge / SafeAutoResolve: creates a merge commit (author=source
//     author, committer=auto-merger), advances jam/<sess>/draft, emits
//     merge.succeeded. If the source commit carries a Resolves-Conflict
//     trailer and the referenced event is open in this session, marks it
//     resolved and emits conflict.resolved.
//   - HardConflict: inserts a conflict_events row, emits conflict.detected.
//     Draft ref is not advanced.
func (a *Applier) Apply(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	switch in.Result.Kind {
	case CleanMerge, SafeAutoResolve:
		return a.applySuccess(ctx, in)
	case HardConflict:
		return a.applyConflict(ctx, in)
	default:
		return ApplyOutput{}, fmt.Errorf("automerger apply: unknown result kind %q", in.Result.Kind)
	}
}

// ---------------------------------------------------------------------------
// Success path
// ---------------------------------------------------------------------------

func (a *Applier) applySuccess(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	sourceCommit, err := object.GetCommit(in.Repo.Storer, in.SourceCommit)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: get source commit: %w", err)
	}
	mergedTree, err := object.GetTree(in.Repo.Storer, plumbing.NewHash(in.Result.MergedTreeSHA))
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: get merged tree: %w", err)
	}

	mergerSig := object.Signature{
		Name:  "jamsesh auto-merger",
		Email: "auto-merger@" + in.PortalHost,
		When:  time.Now().UTC(),
	}

	msg := composeMergeMessage(sourceCommit, in, mergerSig.When)

	mergeCommit := &object.Commit{
		Author:       sourceCommit.Author,
		Committer:    mergerSig,
		Message:      msg,
		TreeHash:     mergedTree.Hash,
		ParentHashes: []plumbing.Hash{in.DraftTip, in.SourceCommit},
	}

	obj := in.Repo.Storer.NewEncodedObject()
	if err := mergeCommit.Encode(obj); err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: encode merge commit: %w", err)
	}
	mergeSHA, err := in.Repo.Storer.SetEncodedObject(obj)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: store merge commit: %w", err)
	}

	// Advance the draft ref.
	draftRefName := plumbing.NewBranchReferenceName("jam/" + in.Session.ID + "/draft")
	ref := plumbing.NewHashReference(draftRefName, mergeSHA)
	if err := in.Repo.Storer.SetReference(ref); err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: advance draft ref: %w", err)
	}

	// Emit merge.succeeded.
	payload := openapi.MergeSucceededPayload{
		SourceSha:      in.SourceCommit.String(),
		DraftSha:       mergeSHA.String(),
		MergeCommitSha: mergeSHA.String(),
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: marshal merge.succeeded payload: %w", err)
	}
	if _, err := a.Log.Emit(ctx, in.Session.OrgID, in.Session.ID, "merge.succeeded", data); err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: emit merge.succeeded: %w", err)
	}

	// Handle Resolves-Conflict trailer on source commit.
	sourceTrailers := prereceive.Trailers(sourceCommit.Message)
	if eventID, ok := sourceTrailers["Resolves-Conflict"]; ok && eventID != "" {
		if err := a.tryResolveConflict(ctx, in, eventID, mergeSHA.String()); err != nil {
			// Log but don't fail the overall Apply — the merge succeeded.
			slog.WarnContext(ctx, "automerger: Resolves-Conflict closure failed",
				"event_id", eventID,
				"session_id", in.Session.ID,
				"err", err,
			)
		}
	}

	if a.Metrics != nil {
		a.Metrics.AutoMergerOutcomes.WithLabelValues("succeeded").Inc()
	}
	return ApplyOutput{MergeCommitSHA: mergeSHA.String()}, nil
}

// composeMergeMessage builds the merge commit message with trailers.
func composeMergeMessage(sourceCommit *object.Commit, in ApplyInput, when time.Time) string {
	_ = when // for future use

	summary := commitSummary(sourceCommit.Message)

	trailers := []string{
		"Auto-Merger: true",
		"Source-Commit: " + in.SourceCommit.String(),
		"Source-Ref: " + in.SourceRef,
	}
	if in.Result.Kind == SafeAutoResolve {
		trailers = append(trailers, "Auto-Resolved: "+in.Result.Heuristic)
	}

	// Propagate Resolves-Conflict from source commit trailers.
	sourceTrailers := prereceive.Trailers(sourceCommit.Message)
	if rc, ok := sourceTrailers["Resolves-Conflict"]; ok && rc != "" {
		trailers = append(trailers, "Resolves-Conflict: "+rc)
	}

	return fmt.Sprintf("Auto-merge: %s\n\n%s\n", summary, strings.Join(trailers, "\n"))
}

// commitSummary extracts the first line (subject) of a commit message.
func commitSummary(message string) string {
	msg := strings.TrimSpace(message)
	if idx := strings.IndexByte(msg, '\n'); idx >= 0 {
		return strings.TrimSpace(msg[:idx])
	}
	return msg
}

// tryResolveConflict attempts to close a conflict event referenced by a
// Resolves-Conflict trailer. Silent no-op when the event-id doesn't match any
// open event for this session; logs a warning when it matches a closed event.
func (a *Applier) tryResolveConflict(ctx context.Context, in ApplyInput, eventID, mergeSHA string) error {
	ev, err := a.Store.GetConflictEventByID(ctx, eventID)
	if err != nil {
		if err == store.ErrNotFound {
			// Unknown event-id — silent no-op.
			return nil
		}
		return fmt.Errorf("get conflict event %s: %w", eventID, err)
	}

	// Scope check: the event must belong to this session.
	if ev.SessionID != in.Session.ID {
		// Not our session — silent no-op.
		return nil
	}

	if ev.Status != "open" {
		// Already resolved. Warn if resolving_commit_sha differs.
		if ev.ResolvingCommitSHA != nil && *ev.ResolvingCommitSHA != mergeSHA {
			slog.WarnContext(ctx, "automerger: Resolves-Conflict references already-closed event with different SHA",
				"event_id", eventID,
				"existing_sha", *ev.ResolvingCommitSHA,
				"new_sha", mergeSHA,
			)
		}
		return nil
	}

	// Mark resolved.
	now := time.Now().UTC()
	if err := a.Store.MarkConflictEventResolved(ctx, store.MarkConflictEventResolvedParams{
		ID:                 eventID,
		SessionID:          in.Session.ID,
		ResolvingCommitSHA: mergeSHA,
		ResolvedAt:         now,
	}); err != nil {
		return fmt.Errorf("mark conflict event resolved: %w", err)
	}

	// Emit conflict.resolved.
	payload := openapi.ConflictResolvedPayload{
		EventId:            eventID,
		ResolvingCommitSha: mergeSHA,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal conflict.resolved payload: %w", err)
	}
	if _, err := a.Log.Emit(ctx, in.Session.OrgID, in.Session.ID, "conflict.resolved", data); err != nil {
		return fmt.Errorf("emit conflict.resolved: %w", err)
	}

	return nil
}

// ---------------------------------------------------------------------------
// Conflict path
// ---------------------------------------------------------------------------

func (a *Applier) applyConflict(ctx context.Context, in ApplyInput) (ApplyOutput, error) {
	addressedTo, err := computeAddressedTo(in.Repo, in.DraftTip, in.Result.Conflicts, in.SourceRef)
	if err != nil {
		// Non-fatal: fall back to source-ref owner only.
		slog.WarnContext(ctx, "automerger: computeAddressedTo failed, using source-ref owner only",
			"session_id", in.Session.ID,
			"err", err,
		)
		if owner := parseSourceRefOwner(in.SourceRef); owner != "" {
			addressedTo = []string{owner}
		}
	}

	// Marshal conflicts and addressed_to as JSON.
	conflictsJSON, err := json.Marshal(in.Result.Conflicts)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: marshal conflicts: %w", err)
	}
	addressedToJSON, err := json.Marshal(addressedTo)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: marshal addressed_to: %w", err)
	}

	now := time.Now().UTC()
	eventID := ulid.Make().String()

	if err := a.Store.InsertConflictEvent(ctx, store.InsertConflictEventParams{
		ID:           eventID,
		OrgID:        in.Session.OrgID,
		SessionID:    in.Session.ID,
		SourceCommit: in.SourceCommit.String(),
		DraftTip:     in.DraftTip.String(),
		Ancestor:     in.Ancestor.String(),
		Conflicts:    string(conflictsJSON),
		AddressedTo:  string(addressedToJSON),
		Status:       "open",
		CreatedAt:    now,
	}); err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: insert conflict event: %w", err)
	}

	// Build openapi ConflictFile list.
	openapiConflicts := make([]openapi.ConflictFile, len(in.Result.Conflicts))
	for i, c := range in.Result.Conflicts {
		ranges := make([]openapi.ConflictFileRange, len(c.Ranges))
		for j, r := range c.Ranges {
			ranges[j] = openapi.ConflictFileRange{Start: r.Start, End: r.End}
		}
		openapiConflicts[i] = openapi.ConflictFile{File: c.File, Ranges: ranges}
	}

	payload := openapi.ConflictDetectedPayload{
		Id:          eventID,
		AddressedTo: addressedTo,
		AncestorSha: in.Ancestor.String(),
		Conflicts:   openapiConflicts,
		CreatedAt:   now,
		DraftTipSha: in.DraftTip.String(),
		Status:      openapi.Open,
	}
	data, err := json.Marshal(payload)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: marshal conflict.detected payload: %w", err)
	}
	if _, err := a.Log.Emit(ctx, in.Session.OrgID, in.Session.ID, "conflict.detected", data); err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: emit conflict.detected: %w", err)
	}

	// Return the freshly inserted event.
	ev, err := a.Store.GetConflictEventByID(ctx, eventID)
	if err != nil {
		return ApplyOutput{}, fmt.Errorf("automerger apply: re-fetch conflict event: %w", err)
	}

	if a.Metrics != nil {
		a.Metrics.AutoMergerOutcomes.WithLabelValues("conflict").Inc()
	}
	return ApplyOutput{ConflictEvent: &ev}, nil
}
