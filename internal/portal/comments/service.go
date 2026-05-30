// Package comments implements the comments REST surface for the portal.
// The Service is exported so mcp-endpoint can call it directly from its
// post_comment / resolve_comment tools.
package comments

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"github.com/oklog/ulid/v2"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/pagination"
)

// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle (comments → playground would be
// cyclic). Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"

// ErrAlreadyResolved is returned by Resolve when the comment is already resolved.
var ErrAlreadyResolved = errors.New("comments: comment already resolved")

// Clock is an injectable time source. Mirrors auth.Clock and tokens.Clock so a
// single *testclock.AdvanceableClock satisfies all of them. Per-package types
// avoid cross-package import coupling — structural typing carries the
// "advance once, move everywhere" property.
type Clock interface {
	Now() time.Time
}

// commentsStore is the minimal store interface consumed by the comments package
// (both Service and Handler).
type commentsStore interface {
	store.CommentStore
	store.SessionStore
	store.SessionMemberStore
	store.PlaygroundSessionStore
	WithTx(ctx context.Context, fn func(store.TxStore) error) error
}

// Service is the business-logic layer for comments.
//
// Service is struct-literal-initialized in cmd/portal/main.go. The Clock field
// is optional: a nil Clock falls back to the real wall clock via the now()
// helper. This preserves backwards compatibility with tests that construct
// Service directly without setting Clock.
type Service struct {
	Store commentsStore
	Log   *events.Log
	Clock Clock
	// PlaygroundIdleTimeout, when > 0, enables activity-reset on successful
	// comment creation for playground sessions: the session's
	// last_substantive_activity_at and idle_timeout_at are bumped to prevent
	// the destruction worker from treating an active session as idle.
	// Zero (default) disables the reset — durable sessions are unaffected
	// regardless of this value (the playground org_id guard fires first).
	PlaygroundIdleTimeout time.Duration
}

// now returns the Service's current time. Falls back to time.Now().UTC() when
// Clock is nil so test code can construct Service literals without
// initializing the field.
func (s *Service) now() time.Time {
	if s.Clock == nil {
		return time.Now().UTC()
	}
	return s.Clock.Now()
}

// CreateParams holds the parameters for creating a comment.
type CreateParams struct {
	OrgID           string
	SessionID       string
	AuthorAccountID string
	AuthorKind      string // "human" | "agent"
	AnchorCommitSHA string
	AnchorFilePath  *string
	AnchorLineStart *int32
	AnchorLineEnd   *int32
	Body            string
	AddressedTo     *string
	Kind            string // "question" | "suggestion" | "action-request" | "fyi"
}

// ResolveParams holds the parameters for resolving a comment.
type ResolveParams struct {
	OrgID           string
	SessionID       string
	CommentID       string
	AccountID       string
	ResolutionNote  *string
}

// ListParams holds the parameters for listing comments.
type ListParams struct {
	OrgID           string
	SessionID       string
	AddressedTo     string // substring filter; empty = no filter
	Kind            string // exact filter; empty = no filter
	Resolved        *bool  // nil = all, true = resolved only, false = unresolved only
	AnchorCommitSHA string // exact filter; empty = no filter
	AnchorFilePath  string // exact filter; empty = no filter
	Cursor          string // opaque cursor from previous response
	Limit           int64
}

// commentAddedPayload is the payload for the comment.added event.
type commentAddedPayload struct {
	ID              string  `json:"id"`
	SessionID       string  `json:"session_id"`
	AuthorID        string  `json:"author_id"`
	AuthorKind      string  `json:"author_kind"`
	AnchorCommitSHA string  `json:"anchor_commit_sha"`
	AnchorFilePath  *string `json:"anchor_file_path,omitempty"`
	AnchorLineStart *int32  `json:"anchor_line_start,omitempty"`
	AnchorLineEnd   *int32  `json:"anchor_line_end,omitempty"`
	Body            string  `json:"body"`
	AddressedTo     *string `json:"addressed_to,omitempty"`
	Kind            string  `json:"kind"`
	CreatedAt       string  `json:"created_at"`
}

// commentResolvedPayload is the payload for the comment.resolved event.
type commentResolvedPayload struct {
	CommentID      string  `json:"comment_id"`
	ResolvedBy     string  `json:"resolved_by"`
	ResolutionNote *string `json:"note,omitempty"`
}

// Create inserts a comment row and emits a comment.added event in one transaction.
func (s *Service) Create(ctx context.Context, p CreateParams) (store.Comment, error) {
	now := s.now()
	id := ulid.Make().String()

	var comment store.Comment
	var allocatedSeq int64
	var allocatedEventID string
	err := s.Store.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.InsertComment(ctx, store.InsertCommentParams{
			ID:              id,
			OrgID:           p.OrgID,
			SessionID:       p.SessionID,
			AuthorAccountID: p.AuthorAccountID,
			AuthorKind:      p.AuthorKind,
			AnchorCommitSHA: p.AnchorCommitSHA,
			AnchorFilePath:  p.AnchorFilePath,
			AnchorLineStart: p.AnchorLineStart,
			AnchorLineEnd:   p.AnchorLineEnd,
			Body:            p.Body,
			AddressedTo:     p.AddressedTo,
			Kind:            p.Kind,
			CreatedAt:       now,
		}); err != nil {
			return fmt.Errorf("insert comment: %w", err)
		}

		payload, err := json.Marshal(commentAddedPayload{
			ID:              id,
			SessionID:       p.SessionID,
			AuthorID:        p.AuthorAccountID,
			AuthorKind:      p.AuthorKind,
			AnchorCommitSHA: p.AnchorCommitSHA,
			AnchorFilePath:  p.AnchorFilePath,
			AnchorLineStart: p.AnchorLineStart,
			AnchorLineEnd:   p.AnchorLineEnd,
			Body:            p.Body,
			AddressedTo:     p.AddressedTo,
			Kind:            p.Kind,
			CreatedAt:       now.Format(time.RFC3339Nano),
		})
		if err != nil {
			return fmt.Errorf("marshal comment.added payload: %w", err)
		}

		if err := tx.EnsureEventSeqRow(ctx, p.SessionID); err != nil {
			return fmt.Errorf("ensure event_seq row: %w", err)
		}
		seq, err := tx.AllocateNextSeq(ctx, p.SessionID)
		if err != nil {
			return fmt.Errorf("allocate seq: %w", err)
		}
		eventID := ulid.Make().String()
		if err := tx.InsertEvent(ctx, store.InsertEventParams{
			ID:        eventID,
			OrgID:     p.OrgID,
			SessionID: p.SessionID,
			Seq:       seq,
			Type:      "comment.added",
			Payload:   string(payload),
			CreatedAt: now,
		}); err != nil {
			return fmt.Errorf("insert comment.added event: %w", err)
		}

		// Capture seq and eventID so the fan-out below can carry the real values.
		allocatedSeq = seq
		allocatedEventID = eventID

		comment = store.Comment{
			ID:              id,
			OrgID:           p.OrgID,
			SessionID:       p.SessionID,
			AuthorAccountID: p.AuthorAccountID,
			AuthorKind:      p.AuthorKind,
			AnchorCommitSHA: p.AnchorCommitSHA,
			AnchorFilePath:  p.AnchorFilePath,
			AnchorLineStart: p.AnchorLineStart,
			AnchorLineEnd:   p.AnchorLineEnd,
			Body:            p.Body,
			AddressedTo:     p.AddressedTo,
			Kind:            p.Kind,
			CreatedAt:       now,
		}
		return nil
	})
	if err != nil {
		return store.Comment{}, err
	}

	// Activity-reset for playground sessions (best-effort). A comment posted
	// by any participant constitutes substantive collaboration and resets the
	// session's idle timer. Durable sessions are unaffected — the org_id guard
	// inside ResetSessionIdleTimer's caller fires first.
	if p.OrgID == playgroundOrgID && s.PlaygroundIdleTimeout > 0 {
		resetAt := s.now()
		if resetErr := s.Store.ResetSessionIdleTimer(ctx, store.ResetSessionIdleTimerParams{
			OrgID:                     p.OrgID,
			SessionID:                 p.SessionID,
			LastSubstantiveActivityAt: resetAt,
			IdleTimeoutAt:             resetAt.Add(s.PlaygroundIdleTimeout),
		}); resetErr != nil {
			// Best-effort: log and continue. The session may die slightly
			// early (original idle_timeout_at), but the comment itself succeeded.
			slog.WarnContext(ctx, "comments: reset idle timer failed (best-effort)",
				"org", p.OrgID, "session", p.SessionID, "err", resetErr)
		}
	}

	// Fan out the comment.added event to WebSocket subscribers.
	// Carry the tx-allocated seq and eventID so the SPA's lastSeenSeq cursor
	// advances correctly and replay dedup works (seq=0 would never advance it).
	payloadBytes, _ := json.Marshal(commentAddedPayload{
		ID:              id,
		SessionID:       p.SessionID,
		AuthorID:        p.AuthorAccountID,
		AuthorKind:      p.AuthorKind,
		AnchorCommitSHA: p.AnchorCommitSHA,
		AnchorFilePath:  p.AnchorFilePath,
		AnchorLineStart: p.AnchorLineStart,
		AnchorLineEnd:   p.AnchorLineEnd,
		Body:            p.Body,
		AddressedTo:     p.AddressedTo,
		Kind:            p.Kind,
		CreatedAt:       now.Format(time.RFC3339Nano),
	})
	s.Log.FanOut(events.Event{
		ID:        allocatedEventID,
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		Seq:       allocatedSeq,
		Type:      "comment.added",
		Payload:   payloadBytes,
		CreatedAt: now,
	})

	return comment, nil
}

// Resolve marks a comment resolved and emits a comment.resolved event.
// Returns ErrAlreadyResolved if the comment is already resolved.
func (s *Service) Resolve(ctx context.Context, p ResolveParams) (store.Comment, error) {
	now := s.now()

	// Load the comment to check current state and retrieve org_id.
	existing, err := s.Store.GetCommentByID(ctx, p.CommentID)
	if err != nil {
		return store.Comment{}, fmt.Errorf("get comment: %w", err)
	}
	if existing.SessionID != p.SessionID {
		return store.Comment{}, store.ErrNotFound
	}
	if existing.ResolvedAt != nil {
		return store.Comment{}, ErrAlreadyResolved
	}

	var resolveSeq int64
	var resolveEventID string
	err = s.Store.WithTx(ctx, func(tx store.TxStore) error {
		if err := tx.ResolveComment(ctx, store.ResolveCommentParams{
			ID:                  p.CommentID,
			SessionID:           p.SessionID,
			ResolvedAt:          now,
			ResolvedByAccountID: p.AccountID,
			ResolutionNote:      p.ResolutionNote,
		}); err != nil {
			return fmt.Errorf("resolve comment: %w", err)
		}

		payload, err := json.Marshal(commentResolvedPayload{
			CommentID:      p.CommentID,
			ResolvedBy:     p.AccountID,
			ResolutionNote: p.ResolutionNote,
		})
		if err != nil {
			return fmt.Errorf("marshal comment.resolved payload: %w", err)
		}

		if err := tx.EnsureEventSeqRow(ctx, p.SessionID); err != nil {
			return fmt.Errorf("ensure event_seq row: %w", err)
		}
		seq, err := tx.AllocateNextSeq(ctx, p.SessionID)
		if err != nil {
			return fmt.Errorf("allocate seq: %w", err)
		}
		eventID := ulid.Make().String()
		if err := tx.InsertEvent(ctx, store.InsertEventParams{
			ID:        eventID,
			OrgID:     p.OrgID,
			SessionID: p.SessionID,
			Seq:       seq,
			Type:      "comment.resolved",
			Payload:   string(payload),
			CreatedAt: now,
		}); err != nil {
			return err
		}
		// Capture seq and eventID for the post-commit fan-out.
		resolveSeq = seq
		resolveEventID = eventID
		return nil
	})
	if err != nil {
		return store.Comment{}, err
	}

	// Return the updated comment.
	resolved := existing
	resolved.ResolvedAt = &now
	resolved.ResolvedByAccountID = &p.AccountID
	resolved.ResolutionNote = p.ResolutionNote

	// Fan out the comment.resolved event to WebSocket subscribers.
	// Carry the tx-allocated seq and eventID so the SPA replay cursor advances.
	payloadBytes, _ := json.Marshal(commentResolvedPayload{
		CommentID:      p.CommentID,
		ResolvedBy:     p.AccountID,
		ResolutionNote: p.ResolutionNote,
	})
	s.Log.FanOut(events.Event{
		ID:        resolveEventID,
		OrgID:     p.OrgID,
		SessionID: p.SessionID,
		Seq:       resolveSeq,
		Type:      "comment.resolved",
		Payload:   payloadBytes,
		CreatedAt: now,
	})

	return resolved, nil
}

// List returns cursor-paginated comments for a session.
func (s *Service) List(ctx context.Context, p ListParams) ([]store.Comment, string, error) {
	const defaultLimit = 20
	const maxLimit = 100

	limit := p.Limit
	if limit <= 0 {
		limit = defaultLimit
	}
	if limit > maxLimit {
		limit = maxLimit
	}

	// Build filter map for cursor validation.
	filter := listFilterMap(p)

	// Decode cursor if provided; first page uses a future sentinel so all rows qualify.
	before := s.now().Add(time.Second)
	// maxIDSentinel sorts after every ULID (26 lowercase base32 chars, max char is 'z')
	// so the keyset condition `id < maxIDSentinel` admits all rows on the first page.
	const maxIDSentinel = "zzzzzzzzzzzzzzzzzzzzzzzzzz"
	lastID := maxIDSentinel
	if p.Cursor != "" {
		cur, err := pagination.Decode(p.Cursor, filter)
		if err != nil {
			return nil, "", fmt.Errorf("decode cursor: %w", err)
		}
		before = cur.LastCreatedAt()
		lastID = cur.LastID
	}

	resolvedFilter := 0
	if p.Resolved != nil {
		if *p.Resolved {
			resolvedFilter = 1
		} else {
			resolvedFilter = 2
		}
	}

	rows, err := s.Store.ListCommentsForSession(ctx, store.ListCommentsForSessionParams{
		SessionID:       p.SessionID,
		AddressedTo:     p.AddressedTo,
		Kind:            p.Kind,
		ResolvedFilter:  resolvedFilter,
		AnchorCommitSHA: p.AnchorCommitSHA,
		AnchorFilePath:  p.AnchorFilePath,
		Before:          before,
		LastID:          lastID,
		Limit:           limit + 1,
	})
	if err != nil {
		return nil, "", fmt.Errorf("list comments: %w", err)
	}

	hasNext := int64(len(rows)) > limit
	if hasNext {
		rows = rows[:limit]
	}

	var nextCursor string
	if hasNext && len(rows) > 0 {
		last := rows[len(rows)-1]
		cur := pagination.NewCursor(last.CreatedAt, last.ID, filter)
		nextCursor = pagination.Encode(cur)
	}

	return rows, nextCursor, nil
}

// listFilterMap builds the filter map for cursor hash computation.
func listFilterMap(p ListParams) map[string]string {
	m := map[string]string{
		"session_id": p.SessionID,
	}
	if p.AddressedTo != "" {
		m["addressed_to"] = p.AddressedTo
	}
	if p.Kind != "" {
		m["kind"] = p.Kind
	}
	if p.Resolved != nil {
		if *p.Resolved {
			m["resolved"] = "true"
		} else {
			m["resolved"] = "false"
		}
	}
	if p.AnchorCommitSHA != "" {
		m["anchor_commit_sha"] = p.AnchorCommitSHA
	}
	if p.AnchorFilePath != "" {
		m["anchor_file_path"] = p.AnchorFilePath
	}
	return m
}
