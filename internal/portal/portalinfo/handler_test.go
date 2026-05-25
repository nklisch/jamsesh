package portalinfo_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/portalinfo"
)

// ---------------------------------------------------------------------------
// StrictServerInterface shim — stubs all methods not under test.
// Follows the strict-server-partial-handler-shim pattern.
// ---------------------------------------------------------------------------

type portalInfoOnlyStrict struct {
	*portalinfo.Handler
}

func (h *portalInfoOnlyStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) Logout(_ context.Context, _ openapi.LogoutRequestObject) (openapi.LogoutResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) IssueWsTicket(_ context.Context, _ openapi.IssueWsTicketRequestObject) (openapi.IssueWsTicketResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetOrg(_ context.Context, _ openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) PatchOrg(_ context.Context, _ openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetFinalizePlan(_ context.Context, _ openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) IssueFetchToken(_ context.Context, _ openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetSessionInvite(_ context.Context, _ openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) MarkSessionShipped(_ context.Context, _ openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) CreatePlaygroundSession(_ context.Context, _ openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetPlaygroundSession(_ context.Context, _ openapi.GetPlaygroundSessionRequestObject) (openapi.GetPlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) JoinPlaygroundSession(_ context.Context, _ openapi.JoinPlaygroundSessionRequestObject) (openapi.JoinPlaygroundSessionResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) GetPlaygroundTombstone(_ context.Context, _ openapi.GetPlaygroundTombstoneRequestObject) (openapi.GetPlaygroundTombstoneResponseObject, error) {
	panic("not wired")
}

// Compile-time assertion that portalInfoOnlyStrict satisfies the full interface.
var _ openapi.StrictServerInterface = (*portalInfoOnlyStrict)(nil)

// ---------------------------------------------------------------------------
// Test environment
// ---------------------------------------------------------------------------

type testEnv struct {
	srv *httptest.Server
}

func newTestEnv(t *testing.T, h *portalinfo.Handler) *testEnv {
	t.Helper()

	strictAPI := openapi.NewStrictHandlerWithOptions(&portalInfoOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictAPI,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	r := chi.NewRouter()
	// Match production wiring: NoCacheMiddleware applied to /portal/info only.
	// (gate-security-portalinfo-no-cachecontrol-no-store)
	r.With(portalinfo.NoCacheMiddleware).Get("/api/portal/info", apiWrapper.GetPortalInfo)

	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return &testEnv{srv: srv}
}

func (e *testEnv) getPortalInfo(t *testing.T) (int, map[string]any) {
	t.Helper()
	resp, err := http.Get(e.srv.URL + "/api/portal/info")
	if err != nil {
		t.Fatalf("GET /api/portal/info: %v", err)
	}
	defer resp.Body.Close()

	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode response body: %v", err)
	}
	return resp.StatusCode, body
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

func TestGetPortalInfo(t *testing.T) {
	cases := []struct {
		name              string
		playgroundEnabled bool
		landingVariant    string
		wantPGEnabled     bool
		wantVariant       string
	}{
		{
			name:              "auto variant, playground enabled",
			playgroundEnabled: true,
			landingVariant:    "auto",
			wantPGEnabled:     true,
			wantVariant:       "auto",
		},
		{
			name:              "auto variant, playground disabled",
			playgroundEnabled: false,
			landingVariant:    "auto",
			wantPGEnabled:     false,
			wantVariant:       "auto",
		},
		{
			name:              "project variant, playground enabled",
			playgroundEnabled: true,
			landingVariant:    "project",
			wantPGEnabled:     true,
			wantVariant:       "project",
		},
		{
			name:              "project variant, playground disabled",
			playgroundEnabled: false,
			landingVariant:    "project",
			wantPGEnabled:     false,
			wantVariant:       "project",
		},
		{
			name:              "login variant, playground enabled",
			playgroundEnabled: true,
			landingVariant:    "login",
			wantPGEnabled:     true,
			wantVariant:       "login",
		},
		{
			name:              "login variant, playground disabled",
			playgroundEnabled: false,
			landingVariant:    "login",
			wantPGEnabled:     false,
			wantVariant:       "login",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h := &portalinfo.Handler{
				PlaygroundEnabled: tc.playgroundEnabled,
				LandingVariant:    tc.landingVariant,
			}
			env := newTestEnv(t, h)

			status, body := env.getPortalInfo(t)

			if status != http.StatusOK {
				t.Fatalf("status: got %d, want 200", status)
			}

			gotPGEnabled, ok := body["playground_enabled"].(bool)
			if !ok {
				t.Fatalf("playground_enabled: missing or wrong type in body %v", body)
			}
			if gotPGEnabled != tc.wantPGEnabled {
				t.Errorf("playground_enabled: got %v, want %v", gotPGEnabled, tc.wantPGEnabled)
			}

			gotVariant, ok := body["landing_variant"].(string)
			if !ok {
				t.Fatalf("landing_variant: missing or wrong type in body %v", body)
			}
			if gotVariant != tc.wantVariant {
				t.Errorf("landing_variant: got %q, want %q", gotVariant, tc.wantVariant)
			}
		})
	}
}

// TestGetPortalInfo_NoAuthRequired confirms the endpoint is reachable without
// any Authorization header.
func TestGetPortalInfo_NoAuthRequired(t *testing.T) {
	h := &portalinfo.Handler{PlaygroundEnabled: false, LandingVariant: "auto"}
	env := newTestEnv(t, h)

	req, err := http.NewRequest(http.MethodGet, env.srv.URL+"/api/portal/info", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// Deliberately omit Authorization header.

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET /api/portal/info: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status: got %d, want 200 (endpoint must be public)", resp.StatusCode)
	}
}

// TestGetPortalInfo_CacheControlNoStore asserts the Cache-Control: no-store
// header is present so deploy-time config flips (PlaygroundEnabled,
// LandingVariant) propagate immediately to all browsers and any
// intermediate cache.
// (gate-security-portalinfo-no-cachecontrol-no-store)
func TestGetPortalInfo_CacheControlNoStore(t *testing.T) {
	h := &portalinfo.Handler{PlaygroundEnabled: true, LandingVariant: "auto"}
	env := newTestEnv(t, h)

	resp, err := http.Get(env.srv.URL + "/api/portal/info")
	if err != nil {
		t.Fatalf("GET /api/portal/info: %v", err)
	}
	defer resp.Body.Close()

	if got := resp.Header.Get("Cache-Control"); got != "no-store" {
		t.Errorf("Cache-Control: got %q, want %q", got, "no-store")
	}
	// The body must still decode normally.
	var body map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&body); err != nil {
		t.Fatalf("decode body: %v", err)
	}
	if _, ok := body["playground_enabled"]; !ok {
		t.Errorf("playground_enabled missing from body: %v", body)
	}
}
