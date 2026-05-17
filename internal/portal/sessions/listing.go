package sessions

import (
	"context"
	"errors"
	"fmt"
	"time"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/pagination"
	"jamsesh/internal/portal/tokens"
)

const defaultPageLimit = 20
const maxPageLimit = 100

// ListSessions — GET /api/orgs/{orgID}/sessions
//
// Returns cursor-paginated sessions for the org. The caller must be an org
// member. Cursor encodes created_at + id + filter hash; a mismatch returns
// 400 pagination.cursor_filter_mismatch.
func (h *Handler) ListSessions(ctx context.Context, req openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.ListSessions401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID

	// Verify org membership.
	if _, err := h.store.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: acc.ID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.ListSessions403JSONResponse{
				ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
					Error:   "auth.insufficient_permission",
					Message: "not a member of this org",
				},
			}, nil
		}
		return nil, fmt.Errorf("sessions: list: get org member: %w", err)
	}

	// Determine page limit (clamped to [1, maxPageLimit]).
	limit := int64(defaultPageLimit)
	if req.Params.Limit > 0 {
		limit = int64(req.Params.Limit)
	}
	if limit > maxPageLimit {
		limit = maxPageLimit
	}

	// Build the filter map for cursor validation.
	filter := map[string]string{
		"org_id": orgID,
	}

	// Decode cursor if provided, otherwise start from "now" (first page).
	before := time.Now().Add(time.Second).UTC() // slight future to include now
	if req.Params.Cursor != "" {
		cur, err := pagination.Decode(req.Params.Cursor, filter)
		if err != nil {
			if errors.Is(err, pagination.ErrFilterMismatch) {
				return openapi.ListSessions400JSONResponse(openapi.ErrorEnvelope{
					Error:   "pagination.cursor_filter_mismatch",
					Message: "cursor does not match current query parameters",
				}), nil
			}
			return openapi.ListSessions400JSONResponse(openapi.ErrorEnvelope{
				Error:   "pagination.invalid_cursor",
				Message: "cursor could not be decoded",
			}), nil
		}
		before = cur.LastCreatedAt()
	}

	// Fetch one extra row to detect whether there's a next page.
	rows, err := h.store.ListSessionsForOrgWithCursor(ctx, store.ListSessionsForOrgWithCursorParams{
		OrgID:  orgID,
		Before: before,
		Limit:  limit + 1,
	})
	if err != nil {
		return nil, fmt.Errorf("sessions: list: query: %w", err)
	}

	hasNext := int64(len(rows)) > limit
	if hasNext {
		rows = rows[:limit]
	}

	items := make([]openapi.Session, len(rows))
	for i, s := range rows {
		members, _ := h.store.ListSessionMembers(ctx, store.ListSessionMembersParams{
			OrgID:     orgID,
			SessionID: s.ID,
		})
		items[i] = sessionToOpenAPI(s, members)
	}

	resp := openapi.SessionListResponse{Items: items}
	if hasNext && len(rows) > 0 {
		last := rows[len(rows)-1]
		cur := pagination.NewCursor(last.CreatedAt, last.ID, filter)
		resp.NextCursor = pagination.Encode(cur)
	}

	return openapi.ListSessions200JSONResponse(resp), nil
}
