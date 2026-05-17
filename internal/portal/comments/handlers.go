package comments

import (
	"context"
	"errors"
	"fmt"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// Handler implements the openapi.StrictServerInterface comment methods.
type Handler struct {
	svc *Service
	s   store.Store
}

// NewHandler constructs a Handler.
func NewHandler(svc *Service) *Handler {
	return &Handler{svc: svc, s: svc.Store}
}

// ---------------------------------------------------------------------------
// ListComments — GET /api/orgs/{orgID}/sessions/{sessionID}/comments
// ---------------------------------------------------------------------------

// ListComments returns cursor-paginated comments for a session.
// The caller must be an org member and a session member.
func (h *Handler) ListComments(ctx context.Context, req openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.ListComments401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID

	// Require org membership.
	if _, err := h.s.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ListComments403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("comments: list: get org member: %w", err)
	}

	// Require session membership.
	if _, err := h.s.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ListComments403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this session",
				},
			}, nil
		}
		return nil, fmt.Errorf("comments: list: get session member: %w", err)
	}

	// Verify session exists in this org.
	if _, err := h.s.GetSession(ctx, orgID, sessionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ListComments404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("comments: list: get session: %w", err)
	}

	// Parse the resolved filter.
	var resolved *bool
	if req.Params.Resolved != "" {
		if string(req.Params.Resolved) == "true" {
			t := true
			resolved = &t
		} else {
			f := false
			resolved = &f
		}
	}

	rows, nextCursor, err := h.svc.List(ctx, ListParams{
		OrgID:           orgID,
		SessionID:       sessionID,
		AddressedTo:     req.Params.AddressedTo,
		Kind:            string(req.Params.Kind),
		Resolved:        resolved,
		AnchorCommitSHA: req.Params.AnchorCommitSha,
		AnchorFilePath:  req.Params.AnchorFilePath,
		Cursor:          req.Params.Cursor,
		Limit:           int64(req.Params.Limit),
	})
	if err != nil {
		// Cursor errors surfaced as 400.
		if isCursorError(err) {
			return openapi.ListComments400JSONResponse(openapi.ErrorEnvelope{
				Error:   "pagination.invalid_cursor",
				Message: "cursor could not be decoded or does not match current filters",
			}), nil
		}
		return nil, fmt.Errorf("comments: list: %w", err)
	}

	items := make([]openapi.Comment, len(rows))
	for i, c := range rows {
		items[i] = storeCommentToAPI(c)
	}

	return openapi.ListComments200JSONResponse(openapi.CommentListResponse{
		Items:      items,
		NextCursor: nextCursor,
	}), nil
}

// ---------------------------------------------------------------------------
// ResolveComment — POST /api/orgs/{orgID}/sessions/{sessionID}/comments/{commentId}/resolve
// ---------------------------------------------------------------------------

// ResolveComment marks a comment resolved.
// The caller must be an org member and a session member.
func (h *Handler) ResolveComment(ctx context.Context, req openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.ResolveComment401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	commentID := req.CommentId

	// Require org membership.
	if _, err := h.s.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ResolveComment403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("comments: resolve: get org member: %w", err)
	}

	// Require session membership.
	if _, err := h.s.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ResolveComment403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this session",
				},
			}, nil
		}
		return nil, fmt.Errorf("comments: resolve: get session member: %w", err)
	}

	var resolutionNote *string
	if req.Body != nil && req.Body.ResolutionNote != "" {
		n := req.Body.ResolutionNote
		resolutionNote = &n
	}

	resolved, err := h.svc.Resolve(ctx, ResolveParams{
		OrgID:          orgID,
		SessionID:      sessionID,
		CommentID:      commentID,
		AccountID:      acc.ID,
		ResolutionNote: resolutionNote,
	})
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ResolveComment404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "not_found",
					Message: "comment not found",
				},
			}, nil
		}
		if errors.Is(err, ErrAlreadyResolved) {
			return openapi.ResolveComment409JSONResponse(openapi.ErrorEnvelope{
				Error:   "comment.already_resolved",
				Message: "comment is already resolved",
			}), nil
		}
		return nil, fmt.Errorf("comments: resolve: %w", err)
	}

	return openapi.ResolveComment200JSONResponse(storeCommentToAPI(resolved)), nil
}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

// storeCommentToAPI converts a store.Comment to an openapi.Comment.
func storeCommentToAPI(c store.Comment) openapi.Comment {
	anchor := openapi.CommentAnchor{
		CommitSha: c.AnchorCommitSHA,
	}
	if c.AnchorFilePath != nil {
		anchor.FilePath = *c.AnchorFilePath
	}
	if c.AnchorLineStart != nil && c.AnchorLineEnd != nil {
		anchor.LineRange = openapi.ConflictFileRange{
			Start: int(*c.AnchorLineStart),
			End:   int(*c.AnchorLineEnd),
		}
	}

	out := openapi.Comment{
		Id:        c.ID,
		SessionId: c.SessionID,
		AuthorId:  c.AuthorAccountID,
		AuthorKind: openapi.CommentAuthorKind(c.AuthorKind),
		Anchor:    anchor,
		Body:      c.Body,
		Kind:      openapi.CommentKind(c.Kind),
		CreatedAt: c.CreatedAt,
	}
	if c.AddressedTo != nil {
		out.AddressedTo = *c.AddressedTo
	}
	if c.ResolvedAt != nil {
		out.ResolvedAt = *c.ResolvedAt
	}
	if c.ResolvedByAccountID != nil {
		out.ResolvedBy = *c.ResolvedByAccountID
	}
	if c.ResolutionNote != nil {
		out.ResolutionNote = *c.ResolutionNote
	}
	return out
}

// isCursorError checks whether an error is a cursor decode/mismatch error.
func isCursorError(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return contains(msg, "decode cursor") || contains(msg, "filter hash")
}

func contains(s, sub string) bool {
	return len(s) >= len(sub) && (s == sub || len(s) > 0 && searchSubstr(s, sub))
}

func searchSubstr(s, sub string) bool {
	for i := 0; i <= len(s)-len(sub); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}
