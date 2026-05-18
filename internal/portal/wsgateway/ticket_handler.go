package wsgateway

import (
	"context"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/tokens"
)

// WsTicketHandler implements the openapi.StrictServerInterface method for
// POST /api/auth/ws-ticket. It issues short-lived single-use upgrade tickets
// that are consumed by the Gateway at WebSocket upgrade time.
type WsTicketHandler struct {
	Tickets *TicketStore
}

// IssueWsTicket implements POST /api/auth/ws-ticket.
// The endpoint requires a valid Bearer token; BearerMiddleware is applied
// upstream by the router so the account is already in context.
func (h *WsTicketHandler) IssueWsTicket(ctx context.Context, _ openapi.IssueWsTicketRequestObject) (openapi.IssueWsTicketResponseObject, error) {
	acct, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.IssueWsTicket401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	tok, ttl, err := h.Tickets.Issue(acct)
	if err != nil {
		// rand.Read failing is a fatal system error; surface as internal.
		return nil, err
	}

	return openapi.IssueWsTicket200JSONResponse{
		Ticket:           tok,
		ExpiresInSeconds: int(ttl.Seconds()),
	}, nil
}
