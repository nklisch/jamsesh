package mcpendpoint

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/modelcontextprotocol/go-sdk/auth"
	"github.com/modelcontextprotocol/go-sdk/mcp"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/comments"
)

// permissionError is a sentinel type for session membership errors.
type permissionError struct{ msg string }

func (e permissionError) Error() string { return e.msg }

func permissionErrorf(format string, args ...any) error {
	return permissionError{msg: fmt.Sprintf(format, args...)}
}

// ---------------------------------------------------------------------------
// post_comment
// ---------------------------------------------------------------------------

// PostCommentInput is the typed input struct for the post_comment tool.
type PostCommentInput struct {
	SessionID   string  `json:"session_id"             jsonschema:"jamsesh session id (uuid)"`
	CommitSHA   string  `json:"commit_sha"             jsonschema:"anchor commit SHA"`
	FilePath    *string `json:"file_path,omitempty"    jsonschema:"optional file path within the commit"`
	LineStart   *int32  `json:"line_start,omitempty"   jsonschema:"optional start line number (1-based)"`
	LineEnd     *int32  `json:"line_end,omitempty"     jsonschema:"optional end line number (inclusive)"`
	Body        string  `json:"body"                   jsonschema:"comment body (markdown)"`
	AddressedTo *string `json:"addressed_to,omitempty" jsonschema:"optional recipient: @agent-id, @all-agents, @everyone, or email"`
	Kind        *string `json:"kind,omitempty"         jsonschema:"comment kind: question, suggestion, action-request, fyi (default: fyi)"`
}

// PostCommentOutput is the typed output for the post_comment tool.
type PostCommentOutput struct {
	CommentID string `json:"comment_id"`
}

// postComment creates a comment in the session via the comments service.
func (e *Endpoint) postComment(
	ctx context.Context, _ *mcp.CallToolRequest, in PostCommentInput,
) (*mcp.CallToolResult, PostCommentOutput, error) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return nil, PostCommentOutput{}, fmt.Errorf("unauthenticated")
	}

	orgID, err := e.findOrg(ctx, info.UserID, in.SessionID)
	if err != nil {
		return nil, PostCommentOutput{}, err
	}

	kind := "fyi"
	if in.Kind != nil && *in.Kind != "" {
		kind = *in.Kind
	}

	c, err := e.Comments.Create(ctx, comments.CreateParams{
		OrgID:           orgID,
		SessionID:       in.SessionID,
		AuthorAccountID: info.UserID,
		AuthorKind:      "agent",
		AnchorCommitSHA: in.CommitSHA,
		AnchorFilePath:  in.FilePath,
		AnchorLineStart: in.LineStart,
		AnchorLineEnd:   in.LineEnd,
		Body:            in.Body,
		AddressedTo:     in.AddressedTo,
		Kind:            kind,
	})
	if err != nil {
		return nil, PostCommentOutput{}, err
	}
	return nil, PostCommentOutput{CommentID: c.ID}, nil
}

// ---------------------------------------------------------------------------
// resolve_comment
// ---------------------------------------------------------------------------

// ResolveCommentInput is the typed input for the resolve_comment tool.
type ResolveCommentInput struct {
	SessionID      string  `json:"session_id"               jsonschema:"jamsesh session id (uuid)"`
	CommentID      string  `json:"comment_id"               jsonschema:"id of the comment to resolve"`
	ResolutionNote *string `json:"resolution_note,omitempty" jsonschema:"optional resolution note"`
}

// ResolveCommentOutput is the typed output for the resolve_comment tool.
type ResolveCommentOutput struct {
	CommentID string `json:"comment_id"`
	Resolved  bool   `json:"resolved"`
}

// resolveComment marks a comment as resolved via the comments service.
func (e *Endpoint) resolveComment(
	ctx context.Context, _ *mcp.CallToolRequest, in ResolveCommentInput,
) (*mcp.CallToolResult, ResolveCommentOutput, error) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return nil, ResolveCommentOutput{}, fmt.Errorf("unauthenticated")
	}

	orgID, err := e.findOrg(ctx, info.UserID, in.SessionID)
	if err != nil {
		return nil, ResolveCommentOutput{}, err
	}

	_, err = e.Comments.Resolve(ctx, comments.ResolveParams{
		OrgID:          orgID,
		SessionID:      in.SessionID,
		CommentID:      in.CommentID,
		AccountID:      info.UserID,
		ResolutionNote: in.ResolutionNote,
	})
	if err != nil {
		return nil, ResolveCommentOutput{}, err
	}
	return nil, ResolveCommentOutput{CommentID: in.CommentID, Resolved: true}, nil
}

// ---------------------------------------------------------------------------
// fork
// ---------------------------------------------------------------------------

// ForkInput is the typed input for the fork tool.
type ForkInput struct {
	SessionID       string  `json:"session_id"              jsonschema:"jamsesh session id (uuid)"`
	TargetCommitSHA string  `json:"target_commit_sha"       jsonschema:"commit SHA to fork from"`
	TargetRef       *string `json:"target_ref,omitempty"    jsonschema:"optional branch name suffix (defaults to fork-<sha7>)"`
	Mode            *string `json:"mode,omitempty"          jsonschema:"collaboration mode: sync or isolated (default: sync)"`
}

// ForkOutput is the typed output for the fork tool.
type ForkOutput struct {
	Ref string `json:"ref"`
	SHA string `json:"sha"`
}

// refForkedPayload matches the event payload shape for ref.forked events.
type refForkedPayload struct {
	SessionID  string `json:"session_id"`
	AccountID  string `json:"account_id"`
	ParentSHA  string `json:"parent_sha"`
	NewRef     string `json:"new_ref"`
	Mode       string `json:"mode"`
	ForkedAt   string `json:"forked_at"`
}

// fork creates an agent branch in the session bare repo pointing at the given
// target commit, upserts the ref_mode, and emits a ref.forked event.
func (e *Endpoint) fork(
	ctx context.Context, _ *mcp.CallToolRequest, in ForkInput,
) (*mcp.CallToolResult, ForkOutput, error) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return nil, ForkOutput{}, fmt.Errorf("unauthenticated")
	}

	orgID, err := e.findOrg(ctx, info.UserID, in.SessionID)
	if err != nil {
		return nil, ForkOutput{}, err
	}

	// Compute the branch name suffix.
	sha7 := in.TargetCommitSHA
	if len(sha7) > 7 {
		sha7 = sha7[:7]
	}
	branchSuffix := "fork-" + sha7
	if in.TargetRef != nil && *in.TargetRef != "" {
		// Strip refs/heads/ prefix if caller supplied a full ref name.
		branchSuffix = strings.TrimPrefix(*in.TargetRef, "refs/heads/")
	}

	// Full ref name under the session namespace.
	refName := plumbing.NewBranchReferenceName(
		fmt.Sprintf("jam/%s/%s/%s", in.SessionID, info.UserID, branchSuffix),
	)

	// Open the bare repo.
	repoPath := e.Storage.RepoPath(orgID, in.SessionID)
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, ForkOutput{}, fmt.Errorf("fork: open repo: %w", err)
	}

	// Verify the target commit exists.
	targetHash := plumbing.NewHash(in.TargetCommitSHA)
	if _, err := repo.CommitObject(targetHash); err != nil {
		return nil, ForkOutput{}, fmt.Errorf("fork: target commit not found: %s", in.TargetCommitSHA)
	}

	// Create (or update) the ref.
	ref := plumbing.NewHashReference(refName, targetHash)
	if err := repo.Storer.SetReference(ref); err != nil {
		return nil, ForkOutput{}, fmt.Errorf("fork: set ref: %w", err)
	}

	// Determine the mode.
	mode := "sync"
	if in.Mode != nil && *in.Mode != "" {
		mode = *in.Mode
	}

	// Upsert the ref_mode row.
	if err := e.Store.UpsertRefMode(ctx, store.UpsertRefModeParams{
		SessionID: in.SessionID,
		Ref:       refName.String(),
		Mode:      mode,
	}); err != nil {
		return nil, ForkOutput{}, fmt.Errorf("fork: upsert ref mode: %w", err)
	}

	// Emit ref.forked event.
	payload, err := json.Marshal(refForkedPayload{
		SessionID: in.SessionID,
		AccountID: info.UserID,
		ParentSHA: in.TargetCommitSHA,
		NewRef:    refName.String(),
		Mode:      mode,
		ForkedAt:  e.now().Format(time.RFC3339Nano),
	})
	if err != nil {
		return nil, ForkOutput{}, fmt.Errorf("fork: marshal event payload: %w", err)
	}
	if _, err := e.Log.Emit(ctx, orgID, in.SessionID, "ref.forked", payload); err != nil {
		// Non-fatal: the ref was created; log the emission failure.
		_ = err
	}

	return nil, ForkOutput{Ref: refName.String(), SHA: in.TargetCommitSHA}, nil
}

// ---------------------------------------------------------------------------
// query_session_state
// ---------------------------------------------------------------------------

// QuerySessionStateInput is the typed input for the query_session_state tool.
type QuerySessionStateInput struct {
	SessionID string   `json:"session_id"          jsonschema:"jamsesh session id (uuid)"`
	SinceSeq  *int64   `json:"since_seq,omitempty" jsonschema:"return events with seq > since_seq (default: 0)"`
	Include   []string `json:"include,omitempty"   jsonschema:"fields to include: goal, scope, draft_tip, unresolved_comments, open_conflicts, recent_events (default: all)"`
}

// EventSummary is a JSON-schema-friendly summary of an event for MCP output.
// It represents events.Event with Payload serialised to a string to keep
// the output schema simple (the SDK validates against a generated schema).
type EventSummary struct {
	ID        string `json:"id"`
	SessionID string `json:"session_id"`
	Seq       int64  `json:"seq"`
	Type      string `json:"type"`
	Payload   string `json:"payload"`
	CreatedAt string `json:"created_at"`
}

// QuerySessionStateOutput is the typed output for the query_session_state tool.
type QuerySessionStateOutput struct {
	Goal               string                `json:"goal,omitempty"`
	Scope              string                `json:"scope,omitempty"`
	DraftTip           string                `json:"draft_tip,omitempty"`
	UnresolvedComments []store.Comment       `json:"unresolved_comments_for_me,omitempty"`
	OpenConflicts      []store.ConflictEvent `json:"open_conflicts_for_me,omitempty"`
	RecentEvents       []EventSummary        `json:"recent_events,omitempty"`
}

const (
	includeGoal        = "goal"
	includeScope       = "scope"
	includeDraftTip    = "draft_tip"
	includeComments    = "unresolved_comments"
	includeConflicts   = "open_conflicts"
	includeEvents      = "recent_events"
	queryEventsLimit   = 100
)

// querySessionState assembles the current state of a session for the caller.
func (e *Endpoint) querySessionState(
	ctx context.Context, _ *mcp.CallToolRequest, in QuerySessionStateInput,
) (*mcp.CallToolResult, QuerySessionStateOutput, error) {
	info := auth.TokenInfoFromContext(ctx)
	if info == nil {
		return nil, QuerySessionStateOutput{}, fmt.Errorf("unauthenticated")
	}

	orgID, err := e.findOrg(ctx, info.UserID, in.SessionID)
	if err != nil {
		return nil, QuerySessionStateOutput{}, err
	}

	// Determine which fields to include. Empty = all.
	include := inclusionSet(in.Include)

	var out QuerySessionStateOutput

	// Fetch the session for goal, scope, and draft_tip.
	if include[includeGoal] || include[includeScope] || include[includeDraftTip] {
		sess, err := e.Store.GetSession(ctx, orgID, in.SessionID)
		if err != nil {
			return nil, QuerySessionStateOutput{}, fmt.Errorf("query_session_state: get session: %w", err)
		}
		if include[includeGoal] {
			out.Goal = sess.Goal
		}
		if include[includeScope] {
			out.Scope = sess.WritableScope
		}
		if include[includeDraftTip] {
			out.DraftTip = e.readDraftTip(orgID, in.SessionID, sess.ID)
		}
	}

	// Fetch unresolved comments addressed to the caller.
	if include[includeComments] {
		// Look up caller's email for the addressed_to filter.
		account, err := e.Store.GetAccountByID(ctx, info.UserID)
		if err != nil {
			return nil, QuerySessionStateOutput{}, fmt.Errorf("query_session_state: get account: %w", err)
		}
		falseVal := false
		unresolvedComments, _, err := e.Comments.List(ctx, comments.ListParams{
			OrgID:       orgID,
			SessionID:   in.SessionID,
			AddressedTo: account.Email,
			Resolved:    &falseVal,
			Limit:       50,
		})
		if err != nil {
			return nil, QuerySessionStateOutput{}, fmt.Errorf("query_session_state: list comments: %w", err)
		}

		// Also fetch comments addressed to @all-agents or @everyone.
		allAgentComments, _, err := e.Comments.List(ctx, comments.ListParams{
			OrgID:       orgID,
			SessionID:   in.SessionID,
			AddressedTo: "@all-agents",
			Resolved:    &falseVal,
			Limit:       50,
		})
		if err == nil {
			unresolvedComments = append(unresolvedComments, allAgentComments...)
		}

		out.UnresolvedComments = unresolvedComments
	}

	// Fetch open conflicts addressed to the caller.
	if include[includeConflicts] {
		conflicts, err := e.Store.ListOpenConflictEventsForSession(ctx, in.SessionID)
		if err != nil {
			return nil, QuerySessionStateOutput{}, fmt.Errorf("query_session_state: list conflicts: %w", err)
		}
		// Filter conflicts addressed to the caller.
		for _, c := range conflicts {
			if isAddressedToAccount(c.AddressedTo, info.UserID) {
				out.OpenConflicts = append(out.OpenConflicts, c)
			}
		}
	}

	// Fetch recent events since the cursor.
	if include[includeEvents] {
		sinceSeq := int64(0)
		if in.SinceSeq != nil {
			sinceSeq = *in.SinceSeq
		}
		evts, err := e.Log.ListSince(ctx, in.SessionID, sinceSeq, queryEventsLimit)
		if err != nil {
			return nil, QuerySessionStateOutput{}, fmt.Errorf("query_session_state: list events: %w", err)
		}
		out.RecentEvents = make([]EventSummary, len(evts))
		for i, ev := range evts {
			out.RecentEvents[i] = EventSummary{
				ID:        ev.ID,
				SessionID: ev.SessionID,
				Seq:       ev.Seq,
				Type:      ev.Type,
				Payload:   string(ev.Payload),
				CreatedAt: ev.CreatedAt.Format(time.RFC3339Nano),
			}
		}
	}

	return nil, out, nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// inclusionSet converts an include list to a lookup map. When the list is
// empty all fields are included.
func inclusionSet(include []string) map[string]bool {
	all := len(include) == 0
	m := map[string]bool{
		includeGoal:      all,
		includeScope:     all,
		includeDraftTip:  all,
		includeComments:  all,
		includeConflicts: all,
		includeEvents:    all,
	}
	for _, f := range include {
		m[f] = true
	}
	return m
}

// readDraftTip reads the SHA of the draft ref (refs/heads/jam/<sessionID>/draft)
// from the bare repo. Returns an empty string if the repo or ref does not exist.
func (e *Endpoint) readDraftTip(orgID, sessionID, _ string) string {
	repoPath := e.Storage.RepoPath(orgID, sessionID)
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return ""
	}
	draftRef := plumbing.NewBranchReferenceName("jam/" + sessionID + "/draft")
	ref, err := repo.Reference(draftRef, true)
	if err != nil {
		return ""
	}
	return ref.Hash().String()
}

// isAddressedToAccount returns true when the addressedTo JSON array includes
// the given accountID, or one of the broadcast targets (@all-agents, @everyone).
func isAddressedToAccount(addressedToJSON, accountID string) bool {
	if addressedToJSON == "" {
		return false
	}
	// addressedTo is stored as a JSON array of strings.
	var targets []string
	if err := json.Unmarshal([]byte(addressedToJSON), &targets); err != nil {
		// Fallback: treat as a plain string for simple cases.
		return strings.Contains(addressedToJSON, accountID) ||
			strings.Contains(addressedToJSON, "@all-agents") ||
			strings.Contains(addressedToJSON, "@everyone")
	}
	for _, t := range targets {
		if t == accountID || t == "@all-agents" || t == "@everyone" {
			return true
		}
	}
	return false
}
