// Package postreceive emits commit.arrived events into the portal event log
// after a git push has been accepted by the pre-receive hook. It is a pure
// emission library: no HTTP handling, no ref validation, no DB schema ownership.
package postreceive

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/gitref"
	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/storage/objectstore"
)

// maxCommitsPerUpdate caps the number of commits processed per ref update on
// a new-ref creation (OldSHA == "") to prevent runaway first-push emission.
const maxCommitsPerUpdate = 1000

// Emitter emits commit.arrived events for accepted ref updates.
// Construct via &Emitter{Log: log}.
//
// In clustered mode, set Syncer to a non-nil *objectstore.Syncer and
// Lifecycle to a non-nil *objectstore.LifecycleManager. EmitForUpdates calls
// Lifecycle.AcquireForRequest to obtain the long-held lease handle, then
// passes it to Syncer.SyncPushPath so the push does not ack until object-
// storage sync completes (RPO=0 contract).
//
// In single-instance mode (Lifecycle == nil, Syncer == nil), the sync step is
// skipped entirely and local disk remains the system of record.
//
// When Syncer is set but Lifecycle is nil (unusual; supported for
// compatibility), a noop handle is used — fencing token is always zero.
type Emitter struct {
	Log *events.Log
	// Syncer is the object-storage sync pipeline. When non-nil, EmitForUpdates
	// calls SyncPushPath after emitting events. When nil, no sync is performed.
	Syncer *objectstore.Syncer
	// Lifecycle is the session lifecycle manager. When non-nil, EmitForUpdates
	// calls AcquireForRequest before SyncPushPath to obtain the long-held lease
	// handle (which performs hydration on first access). In single-instance mode
	// leave nil — the sync step is skipped if Syncer is also nil.
	Lifecycle *objectstore.LifecycleManager
	// Storage is the local-FS storage service used to compute the bare-repo path
	// for the sync call. Required when Syncer is non-nil; unused otherwise.
	Storage storage.Service
}

// RefUpdate is an alias for gitref.RefUpdate. See gitref.RefUpdate for field
// documentation.
type RefUpdate = gitref.RefUpdate

// EmitForUpdates emits a batch of commit.arrived events for every new commit
// in every accepted update. Commits are emitted in chronological order
// (oldest first). Returns nil on success.
//
// session.OrgID is used as the org dimension on every event. The account
// parameter is accepted for API symmetry with the smart-http handler but is
// not consulted — the canonical author_id comes from the Jam-Author trailer
// or commit.Author.Email.
//
// baseSHA is the session's pre-session bootstrap commit (the base ref's
// SHA). Commits reachable from baseSHA are pre-session history and are
// excluded from emission: the walk treats baseSHA as an additional stop
// point alongside update.OldSHA. Pass "" when the session has no base ref
// yet (the very first base-ref push itself stamps it). For a bootstrap
// push where update.NewSHA == baseSHA the walk yields zero new commits
// and no events are emitted.
func (e *Emitter) EmitForUpdates(
	ctx context.Context,
	repo *git.Repository,
	session *store.Session,
	_ *store.Account,
	updates []RefUpdate,
	baseSHA string,
) error {
	for _, update := range updates {
		if err := e.emitForUpdate(ctx, repo, session, update, baseSHA); err != nil {
			return err
		}
	}

	// Clustered mode: mirror push state to object storage before acking the
	// push. This is the RPO=0 contract — the git client does not see a success
	// response until objects + refs are durable in object storage.
	//
	// In single-instance mode (e.Syncer == nil) this block is skipped entirely;
	// local disk remains the system of record with no additional latency.
	if e.Syncer != nil {
		repoPath := e.Storage.RepoPath(session.OrgID, session.ID)

		var handle lease.Handle
		if e.Lifecycle != nil {
			// Clustered mode: acquire the long-held lease handle from the
			// LifecycleManager. This hydrates the local repo if this is the
			// first request for this session on this pod.
			h, err := e.Lifecycle.AcquireForRequest(ctx, session.ID)
			if err != nil {
				return fmt.Errorf("postreceive: lifecycle acquire: %w", err)
			}
			handle = h
			// Note: do NOT release handle here — LifecycleManager owns the
			// handle lifetime. Release is triggered by idle eviction, LRU,
			// lease loss, or shutdown.
		} else {
			// Single-instance fallback or Syncer-without-Lifecycle: use a noop
			// handle (fencing token = 0, Release is a no-op).
			h, err := (lease.NoopManager{}).Acquire(ctx, session.ID)
			if err != nil {
				return fmt.Errorf("postreceive: noop acquire: %w", err)
			}
			defer func() { _ = h.Release() }()
			handle = h
		}

		if _, err := e.Syncer.SyncPushPath(ctx, session.ID, repoPath, handle); err != nil {
			return fmt.Errorf("postreceive: object-storage sync: %w", err)
		}
	}

	return nil
}

// emitForUpdate handles a single ref update: walks new commits, builds
// CommitArrivedPayload drafts (oldest-first), and calls EmitBatch.
//
// The walk stops on any commit in the stop set. update.OldSHA contributes
// when non-empty (subsequent push to existing ref); baseSHA contributes
// when non-empty and different from update.NewSHA (excludes pre-session
// bootstrap history). When both are empty the walk runs unbounded up to
// maxCommitsPerUpdate — the defensive guard for pathological refs whose
// history is not reachable from base.
func (e *Emitter) emitForUpdate(
	ctx context.Context,
	repo *git.Repository,
	session *store.Session,
	update RefUpdate,
	baseSHA string,
) error {
	// No-op: range is empty.
	if update.OldSHA != "" && update.OldSHA == update.NewSHA {
		return nil
	}
	// No-op: bootstrap base-ref push (NewSHA is itself the base) — there
	// are no session-authored commits to emit for the seed.
	if baseSHA != "" && baseSHA == update.NewSHA {
		return nil
	}

	newHash := plumbing.NewHash(update.NewSHA)

	stops := make(map[plumbing.Hash]struct{}, 2)
	if update.OldSHA != "" {
		stops[plumbing.NewHash(update.OldSHA)] = struct{}{}
	}
	if baseSHA != "" && baseSHA != update.NewSHA {
		stops[plumbing.NewHash(baseSHA)] = struct{}{}
	}

	iter, err := repo.Log(&git.LogOptions{From: newHash})
	if err != nil {
		return fmt.Errorf("postreceive: log from %s: %w", update.NewSHA, err)
	}
	defer iter.Close()

	// repo.Log yields commits newest-first. Collect into a slice so we can
	// reverse to chronological (oldest-first) order before emitting.
	var newestFirst []*object.Commit
	iterErr := iter.ForEach(func(c *object.Commit) error {
		if _, isStop := stops[c.Hash]; isStop {
			return storer.ErrStop
		}
		newestFirst = append(newestFirst, c)
		// Cap only applies when no stops are configured (truly unbounded
		// walk — pathological case of a ref whose history doesn't reach
		// base or any prior ref state).
		if len(stops) == 0 && len(newestFirst) >= maxCommitsPerUpdate {
			return storer.ErrStop
		}
		return nil
	})
	if iterErr != nil && iterErr != storer.ErrStop {
		return fmt.Errorf("postreceive: walk commits for %s: %w", update.Ref, iterErr)
	}

	if len(newestFirst) == 0 {
		return nil
	}

	// Build drafts in chronological (oldest-first) order.
	drafts := make([]events.DraftEvent, len(newestFirst))
	for i, c := range newestFirst {
		// Reverse index: element 0 of drafts = last element of newestFirst.
		idx := len(newestFirst) - 1 - i
		payload := openapi.CommitArrivedPayload{
			Sha:      c.Hash.String(),
			Ref:      update.Ref,
			Summary:  commitSummary(c.Message),
			AuthorId: commitAuthorID(c.Message, c.Author.Email),
		}
		raw, err := json.Marshal(payload)
		if err != nil {
			return fmt.Errorf("postreceive: marshal payload for %s: %w", c.Hash.String(), err)
		}
		drafts[idx] = events.DraftEvent{
			Type:    "commit.arrived",
			Payload: json.RawMessage(raw),
		}
	}

	_, err = e.Log.EmitBatch(ctx, session.OrgID, session.ID, drafts)
	return err
}

// commitSummary returns the first line of a commit message, trimmed.
func commitSummary(message string) string {
	if idx := strings.IndexByte(message, '\n'); idx >= 0 {
		return strings.TrimSpace(message[:idx])
	}
	return strings.TrimSpace(message)
}

// commitAuthorID returns the Jam-Author trailer value if present, otherwise
// falls back to the commit's author email.
func commitAuthorID(message, authorEmail string) string {
	trailers := prereceive.Trailers(message)
	if v, ok := trailers["Jam-Author"]; ok && strings.TrimSpace(v) != "" {
		return v
	}
	return authorEmail
}
