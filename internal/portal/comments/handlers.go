package comments

import (
	"context"
	"errors"
	"fmt"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/handlerauth"
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
// CreateComment — POST /api/orgs/{orgID}/sessions/{sessionID}/comments
// ---------------------------------------------------------------------------

// CreateComment inserts a new comment and emits a comment.added event.
func (h *Handler) CreateComment(ctx context.Context, req openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	orgID := req.OrgID
	sessionID := req.SessionID

	// Verify session membership (auth → session member).
	acc, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.s, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, fmt.Errorf("comments: create: %w", fail.Err)
		}
		return createCommentFail(fail), nil
	}

	if req.Body == nil {
		return openapi.CreateComment400JSONResponse(openapi.ErrorEnvelope{
			Error:   "request.invalid",
			Message: "request body is required",
		}), nil
	}

	body := req.Body

	// Verify session exists.
	if _, err := h.s.GetSession(ctx, orgID, sessionID); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.CreateComment404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "session.not_found",
					Message: "session not found",
				},
			}, nil
		}
		return nil, fmt.Errorf("comments: create: get session: %w", err)
	}

	// Build optional pointer fields.
	var anchorFilePath *string
	if body.AnchorFilePath != "" {
		s := body.AnchorFilePath
		anchorFilePath = &s
	}
	var anchorLineStart, anchorLineEnd *int32
	if body.AnchorLineStart != 0 {
		v := int32(body.AnchorLineStart)
		anchorLineStart = &v
	}
	if body.AnchorLineEnd != 0 {
		v := int32(body.AnchorLineEnd)
		anchorLineEnd = &v
	}
	var addressedTo *string
	if body.AddressedTo != "" {
		s := body.AddressedTo
		addressedTo = &s
	}

	comment, err := h.svc.Create(ctx, CreateParams{
		OrgID:           orgID,
		SessionID:       sessionID,
		AuthorAccountID: acc.ID,
		AuthorKind:      "human",
		AnchorCommitSHA: body.AnchorCommitSha,
		AnchorFilePath:  anchorFilePath,
		AnchorLineStart: anchorLineStart,
		AnchorLineEnd:   anchorLineEnd,
		Body:            body.Body,
		AddressedTo:     addressedTo,
		Kind:            string(body.Kind),
	})
	if err != nil {
		return nil, fmt.Errorf("comments: create: %w", err)
	}

	return openapi.CreateComment201JSONResponse(storeCommentToAPI(comment)), nil
}

// ---------------------------------------------------------------------------
// ListComments — GET /api/orgs/{orgID}/sessions/{sessionID}/comments
// ---------------------------------------------------------------------------

// ListComments returns cursor-paginated comments for a session.
// The caller must be a session member.
func (h *Handler) ListComments(ctx context.Context, req openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	orgID := req.OrgID
	sessionID := req.SessionID

	// Verify session membership (auth → session member).
	_, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.s, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, fmt.Errorf("comments: list: %w", fail.Err)
		}
		return listCommentsFail(fail), nil
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
// The caller must be a session member.
func (h *Handler) ResolveComment(ctx context.Context, req openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	orgID := req.OrgID
	sessionID := req.SessionID
	commentID := req.CommentId

	// Verify session membership (auth → session member).
	acc, _, fail, ok := handlerauth.RequireSessionMember(ctx, h.s, orgID, sessionID)
	if !ok {
		if fail.Err != nil {
			return nil, fmt.Errorf("comments: resolve: %w", fail.Err)
		}
		return resolveCommentFail(fail), nil
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

// ---------------------------------------------------------------------------
// Per-handler auth-fail wrappers
//
// Each function wraps a handlerauth.AuthFail into the operation-specific
// response type required by oapi-codegen's strict-server interface.
// ---------------------------------------------------------------------------

func createCommentFail(f handlerauth.AuthFail) openapi.CreateCommentResponseObject {
	if f.Status == 401 {
		return openapi.CreateComment401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.CreateComment403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func listCommentsFail(f handlerauth.AuthFail) openapi.ListCommentsResponseObject {
	if f.Status == 401 {
		return openapi.ListComments401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.ListComments403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

func resolveCommentFail(f handlerauth.AuthFail) openapi.ResolveCommentResponseObject {
	if f.Status == 401 {
		return openapi.ResolveComment401JSONResponse{UnauthorizedJSONResponse: f.Unauthorized}
	}
	return openapi.ResolveComment403JSONResponse{ForbiddenJSONResponse: f.Forbidden}
}

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
