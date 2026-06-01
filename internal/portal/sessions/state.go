package sessions

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"strings"

	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/storer"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/handlerauth"
)

// GetSession — GET /api/orgs/{orgID}/sessions/{sessionID}
func (h *Handler) GetSession(ctx context.Context, req openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	orgID := req.OrgID
	sessionID := req.SessionID

	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get: session member: %w", fail.Err))
		}
		return getSessionFail(fail), nil
	}

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetSession404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: get: %w", err))
	}

	members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
		OrgID:     orgID,
		SessionID: sessionID,
	})

	return openapi.GetSession200JSONResponse(sessionToOpenAPI(sess, members)), nil
}

// ListSessionRefs — GET /api/orgs/{orgID}/sessions/{sessionID}/refs
//
// Opens the bare repo via storage, iterates all refs under refs/heads/jam/,
// and annotates each with its collaboration mode (per-ref override from
// ref_modes table, or session default_mode).
func (h *Handler) ListSessionRefs(ctx context.Context, req openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	orgID := req.OrgID
	sessionID := req.SessionID

	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: list refs: session member: %w", fail.Err))
		}
		return listSessionRefsFail(fail), nil
	}

	// Fetch the session for default_mode.
	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ListSessionRefs404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: list refs: get session: %w", err))
	}

	// Load all per-ref mode overrides for this session.
	refModeRows, err := h.store.ListRefModesForSession(ctx, sessionID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: list refs: ref modes: %w", err))
	}
	refModeMap := make(map[string]string, len(refModeRows))
	for _, rm := range refModeRows {
		refModeMap[rm.Ref] = rm.Mode
	}

	// Open the bare repo.
	repoPath := h.storage.RepoPath(orgID, sessionID)
	repo, err := git.PlainOpen(repoPath)
	if err != nil {
		// Repo may not exist yet (no pushes). Return empty list.
		return openapi.ListSessionRefs200JSONResponse(openapi.RefListResponse{
			Refs: []openapi.Ref{},
		}), nil
	}

	// Iterate refs, filtering to the session namespace.
	prefix := "refs/heads/jam/" + sessionID + "/"
	iter, err := repo.References()
	if err != nil {
		return nil, fmt.Errorf("sessions: list refs: iterate refs: %w", err)
	}
	defer iter.Close()

	var refs []openapi.Ref
	iterErr := iter.ForEach(func(r *plumbing.Reference) error {
		if r.Type() == plumbing.SymbolicReference {
			return nil // skip HEAD etc.
		}
		if !strings.HasPrefix(r.Name().String(), prefix) {
			return nil // skip refs outside this session
		}

		// Determine mode: per-ref override, or session default.
		mode := sess.DefaultMode
		if override, ok := refModeMap[r.Name().String()]; ok {
			mode = override
		}

		refs = append(refs, openapi.Ref{
			Ref:  r.Name().String(),
			Sha:  r.Hash().String(),
			Mode: openapi.RefMode(mode),
		})
		return nil
	})
	if iterErr != nil && !errors.Is(iterErr, storer.ErrStop) {
		return nil, fmt.Errorf("sessions: list refs: iterate: %w", iterErr)
	}

	if refs == nil {
		refs = []openapi.Ref{}
	}

	return openapi.ListSessionRefs200JSONResponse(openapi.RefListResponse{Refs: refs}), nil
}

// GetSessionDigest — GET /api/orgs/{orgID}/sessions/{sessionID}/digest?since=<seq>
//
// Assembles a plain-text turn-start digest from digest-relevant events since
// cursor. Returns {text, next_cursor} where next_cursor is the max seq seen.
func (h *Handler) GetSessionDigest(ctx context.Context, req openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	orgID := req.OrgID
	sessionID := req.SessionID
	sinceSeq := req.Params.Since // zero means "from beginning"

	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.store, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: digest: session member: %w", fail.Err))
		}
		return getSessionDigestFail(fail), nil
	}

	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetSessionDigest404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: digest: get session: %w", err))
	}

	const digestEventLimit = 500
	events, err := h.store.ListEventsSinceForDigest(ctx, store.ListEventsSinceForDigestParams{
		SessionID: sessionID,
		SinceSeq:  sinceSeq,
		Limit:     digestEventLimit,
	})
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("sessions: digest: list events: %w", err))
	}

	// Find the max seq seen.
	var nextCursor int64 = sinceSeq
	for _, e := range events {
		if e.Seq > nextCursor {
			nextCursor = e.Seq
		}
	}

	text := assembleDigest(sess, events)

	return openapi.GetSessionDigest200JSONResponse(openapi.DigestResponse{
		Text:       text,
		NextCursor: nextCursor,
	}), nil
}

// ---------------------------------------------------------------------------
// Digest assembly
// ---------------------------------------------------------------------------

// assembleDigest formats a plain-text turn-start digest per PROTOCOL.md.
// Sections: peer commits, comments, conflicts, mode changes, state summary.
func assembleDigest(sess store.Session, events []store.Event) string {
	var sb strings.Builder

	sb.WriteString("# jamsesh digest for ")
	sb.WriteString(sess.Name)
	sb.WriteString("\n\n")

	// Group events by type.
	type commitEntry struct{ ref, sha, authorID, summary string }
	type commentEntry struct{ body, addressedTo, kind string }
	type conflictEntry struct{ id, sourceRef, status string }
	type modeEntry struct{ ref, oldMode, newMode string }

	var commits []commitEntry
	var comments []commentEntry
	var resolvedComments []commentEntry
	var conflicts []conflictEntry
	var modeChanges []modeEntry

	for _, e := range events {
		switch e.Type {
		case "commit.arrived":
			var p struct {
				Ref      string `json:"ref"`
				Sha      string `json:"sha"`
				AuthorID string `json:"author_id"`
				Summary  string `json:"summary"`
			}
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				commits = append(commits, commitEntry{ref: p.Ref, sha: p.Sha, authorID: p.AuthorID, summary: p.Summary})
			}
		case "comment.added":
			var p struct {
				Body        string `json:"body"`
				AddressedTo string `json:"addressed_to"`
				Kind        string `json:"kind"`
			}
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				comments = append(comments, commentEntry{body: p.Body, addressedTo: p.AddressedTo, kind: p.Kind})
			}
		case "comment.resolved":
			var p struct {
				Note string `json:"note"`
			}
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				resolvedComments = append(resolvedComments, commentEntry{body: p.Note})
			}
		case "conflict.detected":
			var p struct {
				ID        string `json:"id"`
				SourceRef string `json:"source_ref"`
				Status    string `json:"status"`
			}
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				conflicts = append(conflicts, conflictEntry{id: p.ID, sourceRef: p.SourceRef, status: p.Status})
			}
		case "conflict.resolved":
			var p struct {
				EventID string `json:"event_id"`
			}
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				// Mark as resolved in the conflicts list if found.
				for i := range conflicts {
					if conflicts[i].id == p.EventID {
						conflicts[i].status = "resolved"
					}
				}
			}
		case "mode.changed":
			var p struct {
				Ref     string `json:"ref"`
				OldMode string `json:"old_mode"`
				NewMode string `json:"new_mode"`
			}
			if err := json.Unmarshal([]byte(e.Payload), &p); err == nil {
				modeChanges = append(modeChanges, modeEntry{ref: p.Ref, oldMode: p.OldMode, newMode: p.NewMode})
			}
		}
	}

	// Section: peer commits.
	sb.WriteString("## peer activity\n")
	if len(commits) == 0 {
		sb.WriteString("(no peer commits since last turn)\n")
	} else {
		for _, c := range commits {
			sha := c.sha
			if len(sha) > 12 {
				sha = sha[:12]
			}
			sb.WriteString(fmt.Sprintf("- [%s] %s (%s) — %s\n", sha, c.ref, c.authorID, c.summary))
		}
	}
	sb.WriteString("\n")

	// Section: comments.
	sb.WriteString("## comments\n")
	if len(comments) == 0 && len(resolvedComments) == 0 {
		sb.WriteString("(no comments)\n")
	} else {
		for _, c := range comments {
			addr := ""
			if c.addressedTo != "" {
				addr = " → " + c.addressedTo
			}
			sb.WriteString(fmt.Sprintf("- [%s%s] %s\n", c.kind, addr, c.body))
		}
		for _, c := range resolvedComments {
			note := c.body
			if note == "" {
				note = "(resolved)"
			}
			sb.WriteString(fmt.Sprintf("- [resolved] %s\n", note))
		}
	}
	sb.WriteString("\n")

	// Section: conflicts.
	sb.WriteString("## conflicts\n")
	var openConflicts []conflictEntry
	for _, c := range conflicts {
		if c.status != "resolved" {
			openConflicts = append(openConflicts, c)
		}
	}
	if len(openConflicts) == 0 {
		sb.WriteString("(none)\n")
	} else {
		for _, c := range openConflicts {
			sb.WriteString(fmt.Sprintf("- conflict %s on %s [%s]\n", c.id[:8], c.sourceRef, c.status))
		}
	}
	sb.WriteString("\n")

	// Section: mode changes.
	if len(modeChanges) > 0 {
		sb.WriteString("## mode changes\n")
		for _, m := range modeChanges {
			sb.WriteString(fmt.Sprintf("- %s: %s → %s\n", m.ref, m.oldMode, m.newMode))
		}
		sb.WriteString("\n")
	}

	// Section: current state.
	sb.WriteString("## state\n")
	sb.WriteString(fmt.Sprintf("- goal: %s\n", sess.Goal))
	sb.WriteString(fmt.Sprintf("- status: %s\n", sess.Status))
	if sess.BaseSHA != nil {
		sb.WriteString(fmt.Sprintf("- base: %s\n", *sess.BaseSHA))
	}
	sb.WriteString(fmt.Sprintf("- default mode: %s\n", sess.DefaultMode))

	return sb.String()
}

// Ensure storer is imported via its ErrStop usage.
var _ = storer.ErrStop
