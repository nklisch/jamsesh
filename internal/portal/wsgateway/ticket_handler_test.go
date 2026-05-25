package wsgateway_test

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
	"jamsesh/internal/portal/wsgateway"
)

// ---------------------------------------------------------------------------
// WsTicketHandler tests
// ---------------------------------------------------------------------------

// ticketHandlerEnv holds a wired-up httptest server for ticket-handler tests.
type ticketHandlerEnv struct {
	store   store.Store
	svc     tokens.Service
	tickets *wsgateway.TicketStore
	srv     *httptest.Server
}

// wsTicketOnlyHandler implements openapi.StrictServerInterface, routing only
// IssueWsTicket. All other methods panic (they are not called in these tests).
type wsTicketOnlyHandler struct {
	*wsgateway.WsTicketHandler
}

// Implement every method of StrictServerInterface except IssueWsTicket by
// panicking — the test router only mounts /api/auth/ws-ticket.

func (h *wsTicketOnlyHandler) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetOrg(_ context.Context, _ openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) PatchOrg(_ context.Context, _ openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetFinalizePlan(_ context.Context, _ openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) IssueFetchToken(_ context.Context, _ openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) GetSessionInvite(_ context.Context, _ openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) MarkSessionShipped(_ context.Context, _ openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("not implemented")
}
func (h *wsTicketOnlyHandler) CreatePlaygroundSession(_ context.Context, _ openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *wsTicketOnlyHandler) JoinPlaygroundSession(_ context.Context, _ openapi.JoinPlaygroundSessionRequestObject) (openapi.JoinPlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *wsTicketOnlyHandler) GetPlaygroundSession(_ context.Context, _ openapi.GetPlaygroundSessionRequestObject) (openapi.GetPlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *wsTicketOnlyHandler) GetPlaygroundTombstone(_ context.Context, _ openapi.GetPlaygroundTombstoneRequestObject) (openapi.GetPlaygroundTombstoneResponseObject, error) {
	panic("not wired")
}
func (h *wsTicketOnlyHandler) GetPortalInfo(_ context.Context, _ openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
	panic("not wired")
}

// Ensure wsTicketOnlyHandler satisfies the interface at compile time.
var _ openapi.StrictServerInterface = (*wsTicketOnlyHandler)(nil)

// newTicketHandlerEnv builds an httptest server that mirrors the real
// portal's route split for the ws-ticket endpoint:
//   - POST /api/auth/ws-ticket behind BearerMiddleware
func newTicketHandlerEnv(t *testing.T) *ticketHandlerEnv {
	t.Helper()

	s := openStore(t)
	svc := tokens.New(s)
	ticketStore := wsgateway.NewTicketStore()
	ticketStore.Start()

	h := &wsTicketOnlyHandler{
		WsTicketHandler: &wsgateway.WsTicketHandler{Tickets: ticketStore},
	}
	strictAPI := openapi.NewStrictHandler(h, nil)

	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(svc))
		r.Post("/api/auth/ws-ticket", strictAPI.IssueWsTicket)
	})

	srv := httptest.NewServer(r)
	t.Cleanup(func() {
		srv.Close()
		ticketStore.Stop()
	})

	return &ticketHandlerEnv{
		store:   s,
		svc:     svc,
		tickets: ticketStore,
		srv:     srv,
	}
}

// issueAccessToken issues a bearer token pair for the given account ID.
func issueAccessToken(t *testing.T, svc tokens.Service, accountID string) string {
	t.Helper()
	pair, err := svc.Issue(context.Background(), accountID)
	if err != nil {
		t.Fatalf("Issue token: %v", err)
	}
	return pair.AccessToken
}

// postWsTicket sends a POST /api/auth/ws-ticket with the given bearer token.
func postWsTicket(t *testing.T, srv *httptest.Server, bearerToken string) *http.Response {
	t.Helper()
	req, err := http.NewRequest(http.MethodPost, srv.URL+"/api/auth/ws-ticket", bytes.NewReader([]byte("{}")))
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	if bearerToken != "" {
		req.Header.Set("Authorization", "Bearer "+bearerToken)
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("do request: %v", err)
	}
	return resp
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

// TestIssueWsTicket_401WithoutBearer verifies that the endpoint returns 401
// when no Authorization header is provided.
func TestIssueWsTicket_401WithoutBearer(t *testing.T) {
	env := newTicketHandlerEnv(t)
	resp := postWsTicket(t, env.srv, "")
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Errorf("want 401 without bearer, got %d", resp.StatusCode)
	}
}

// TestIssueWsTicket_200WithBearer verifies that an authenticated request
// returns a ticket with the expected shape and TTL.
func TestIssueWsTicket_200WithBearer(t *testing.T) {
	env := newTicketHandlerEnv(t)

	// Create an account and issue a bearer token.
	acc, err := env.store.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          "acc-ticket-1",
		Email:       "ticket1@example.com",
		DisplayName: "Ticket Test",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	bearer := issueAccessToken(t, env.svc, acc.ID)

	resp := postWsTicket(t, env.srv, bearer)
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("want 200, got %d", resp.StatusCode)
	}

	var body struct {
		Ticket          string `json:"ticket"`
		ExpiresInSeconds int   `json:"expires_in_seconds"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}
	if body.Ticket == "" {
		t.Error("ticket is empty")
	}
	if body.ExpiresInSeconds != 60 {
		t.Errorf("expires_in_seconds: want 60, got %d", body.ExpiresInSeconds)
	}
}

// TestIssueWsTicket_TicketIsConsumable verifies that the issued ticket can be
// consumed from the ticket store (i.e. it is actually stored there).
func TestIssueWsTicket_TicketIsConsumable(t *testing.T) {
	env := newTicketHandlerEnv(t)

	acc, err := env.store.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          "acc-ticket-2",
		Email:       "ticket2@example.com",
		DisplayName: "Ticket Test 2",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	bearer := issueAccessToken(t, env.svc, acc.ID)

	resp := postWsTicket(t, env.srv, bearer)
	defer resp.Body.Close()

	var body struct {
		Ticket string `json:"ticket"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response: %v", err)
	}

	// The ticket must be consumable from the store (proves it was persisted).
	got := env.tickets.Consume(body.Ticket)
	if got == nil {
		t.Error("Consume returned nil — ticket was not stored or already expired")
	}
	if got != nil && got.ID != acc.ID {
		t.Errorf("consumed account ID: want %s, got %s", acc.ID, got.ID)
	}
}

// TestIssueWsTicket_EachCallReturnsDifferentTicket verifies that multiple
// calls each return a distinct ticket.
func TestIssueWsTicket_EachCallReturnsDifferentTicket(t *testing.T) {
	env := newTicketHandlerEnv(t)

	acc, err := env.store.CreateAccount(context.Background(), store.CreateAccountParams{
		ID:          "acc-ticket-3",
		Email:       "ticket3@example.com",
		DisplayName: "Ticket Test 3",
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("CreateAccount: %v", err)
	}
	bearer := issueAccessToken(t, env.svc, acc.ID)

	var tickets []string
	for i := 0; i < 5; i++ {
		resp := postWsTicket(t, env.srv, bearer)
		var body struct {
			Ticket string `json:"ticket"`
		}
		if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
			resp.Body.Close()
			t.Fatalf("decode response %d: %v", i, err)
		}
		resp.Body.Close()
		tickets = append(tickets, body.Ticket)
	}

	seen := make(map[string]struct{})
	for _, tok := range tickets {
		if _, dup := seen[tok]; dup {
			t.Errorf("duplicate ticket returned: %s", tok)
		}
		seen[tok] = struct{}{}
	}
}
