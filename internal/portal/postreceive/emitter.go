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
	"jamsesh/internal/portal/prereceive"
)

// maxCommitsPerUpdate caps the number of commits processed per ref update on
// a new-ref creation (OldSHA == "") to prevent runaway first-push emission.
const maxCommitsPerUpdate = 1000

// Emitter emits commit.arrived events for accepted ref updates.
// Construct via &Emitter{Log: log}.
type Emitter struct {
	Log *events.Log
}

// RefUpdate describes one ref that was accepted during a push.
// OldSHA is empty when the ref is being created for the first time.
type RefUpdate struct {
	Ref    string // e.g. "refs/heads/jam/<session>/<owner>/<branch>"
	OldSHA string // empty if new ref
	NewSHA string
}

// EmitForUpdates emits a batch of commit.arrived events for every new commit
// in every accepted update. Commits are emitted in chronological order
// (oldest first). Returns nil on success.
//
// session.OrgID is used as the org dimension on every event. The account
// parameter is accepted for API symmetry with the smart-http handler but is
// not consulted — the canonical author_id comes from the Jam-Author trailer
// or commit.Author.Email.
func (e *Emitter) EmitForUpdates(
	ctx context.Context,
	repo *git.Repository,
	session *store.Session,
	_ *store.Account,
	updates []RefUpdate,
) error {
	for _, update := range updates {
		if err := e.emitForUpdate(ctx, repo, session, update); err != nil {
			return err
		}
	}
	return nil
}

// emitForUpdate handles a single ref update: walks new commits, builds
// CommitArrivedPayload drafts (oldest-first), and calls EmitBatch.
func (e *Emitter) emitForUpdate(
	ctx context.Context,
	repo *git.Repository,
	session *store.Session,
	update RefUpdate,
) error {
	// No-op: range is empty.
	if update.OldSHA != "" && update.OldSHA == update.NewSHA {
		return nil
	}

	newHash := plumbing.NewHash(update.NewSHA)

	hasStop := update.OldSHA != ""
	stopHash := plumbing.NewHash(update.OldSHA) // zero hash when !hasStop

	iter, err := repo.Log(&git.LogOptions{From: newHash})
	if err != nil {
		return fmt.Errorf("postreceive: log from %s: %w", update.NewSHA, err)
	}
	defer iter.Close()

	// repo.Log yields commits newest-first. Collect into a slice so we can
	// reverse to chronological (oldest-first) order before emitting.
	var newestFirst []*object.Commit
	iterErr := iter.ForEach(func(c *object.Commit) error {
		if hasStop && c.Hash == stopHash {
			return storer.ErrStop
		}
		newestFirst = append(newestFirst, c)
		if !hasStop && len(newestFirst) >= maxCommitsPerUpdate {
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
