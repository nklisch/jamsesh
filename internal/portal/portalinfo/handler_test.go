package portalinfo_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
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
func (h *portalInfoOnlyStrict) CreateSessionResume(_ context.Context, _ openapi.CreateSessionResumeRequestObject) (openapi.CreateSessionResumeResponseObject, error) {
	panic("not wired")
}
func (h *portalInfoOnlyStrict) ExchangeSessionResume(_ context.Context, _ openapi.ExchangeSessionResumeRequestObject) (openapi.ExchangeSessionResumeResponseObject, error) {
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

// TestGetPortalInfo_InvalidLandingVariantSurfacesError is a
// defense-in-depth regression: every value held on the Handler must
// satisfy openapi.PortalInfoLandingVariant.Valid() — and the only
// production path to constructing one is portalinfo.NewHandler, which
// refuses invalid input.
//
// The valid subcases assert NewHandler succeeds, returns a populated
// *Handler, and the response round-trips the variant untouched. The
// invalid subcases assert NewHandler returns a typed error whose
// message quotes the offending value, and the returned *Handler is nil
// so no downstream caller can accidentally use a half-built handler.
//
// Story: gate-tests-portalinfo-handler-invalid-enum-defense
func TestGetPortalInfo_InvalidLandingVariantSurfacesError(t *testing.T) {
	cases := []struct {
		name           string
		landingVariant string
		valid          bool // is this a known PortalInfoLandingVariant member?
	}{
		{"valid auto", "auto", true},
		{"valid login", "login", true},
		{"valid project", "project", true},
		{"invalid empty", "", false},
		{"invalid garbage", "not-a-real-variant", false},
		{"invalid case mismatch", "Auto", false},
		{"invalid whitespace", "auto ", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			h, err := portalinfo.NewHandler(true, tc.landingVariant)

			if !tc.valid {
				// Invalid input must surface a typed error and return a
				// nil handler so no caller can accidentally use it.
				if err == nil {
					t.Fatalf("NewHandler(%q) returned nil error; want validation failure", tc.landingVariant)
				}
				if !strings.Contains(err.Error(), fmt.Sprintf("%q", tc.landingVariant)) {
					t.Errorf("NewHandler(%q) error message %q does not quote the invalid value",
						tc.landingVariant, err.Error())
				}
				if h != nil {
					t.Errorf("NewHandler(%q) returned non-nil *Handler on validation failure: %+v",
						tc.landingVariant, h)
				}
				return
			}

			// Valid input must succeed.
			if err != nil {
				t.Fatalf("NewHandler(%q) returned error: %v", tc.landingVariant, err)
			}
			if h == nil {
				t.Fatalf("NewHandler(%q) returned nil *Handler with nil error", tc.landingVariant)
			}

			// 1. Direct call: a valid landing variant must round-trip
			//    untouched and satisfy Valid() on the wire.
			resp, err := h.GetPortalInfo(
				context.Background(),
				openapi.GetPortalInfoRequestObject{},
			)
			if err != nil {
				t.Fatalf("GetPortalInfo(%q) returned error: %v", tc.landingVariant, err)
			}
			ok, isOK := resp.(openapi.GetPortalInfo200JSONResponse)
			if !isOK {
				t.Fatalf("GetPortalInfo(%q) returned %T, want GetPortalInfo200JSONResponse",
					tc.landingVariant, resp)
			}
			if !ok.LandingVariant.Valid() {
				t.Errorf(
					"LandingVariant(%q).Valid() = false; openapi enum members are %q/%q/%q",
					tc.landingVariant, openapi.Auto, openapi.Login, openapi.Project,
				)
			}

			// 2. HTTP round-trip: the JSON body carries the same value.
			env := newTestEnv(t, h)
			status, body := env.getPortalInfo(t)
			if status != http.StatusOK {
				t.Fatalf("getPortalInfo: status=%d, want 200", status)
			}
			gotVariant, _ := body["landing_variant"].(string)
			if gotVariant != tc.landingVariant {
				t.Errorf("landing_variant: got %q, want %q",
					gotVariant, tc.landingVariant)
			}
		})
	}
}

// TestGetPortalInfo_WrongMethodReturns405 asserts that POST/PUT/DELETE on
// /api/portal/info yields 405 Method Not Allowed (NOT 200, NOT 404).
// /api/portal/info is the SPA's bootstrap endpoint — a write verb
// accidentally accepted would be a real bug surface, even if the handler
// is harmless today, because the route would be silently malformed.
//
// Story: gate-tests-portalinfo-method-not-allowed-cors
func TestGetPortalInfo_WrongMethodReturns405(t *testing.T) {
	h := &portalinfo.Handler{PlaygroundEnabled: true, LandingVariant: "auto"}
	env := newTestEnv(t, h)

	methods := []string{http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			req, err := http.NewRequest(method, env.srv.URL+"/api/portal/info", nil)
			if err != nil {
				t.Fatalf("new request: %v", err)
			}
			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("%s /api/portal/info: %v", method, err)
			}
			defer resp.Body.Close()

			if resp.StatusCode != http.StatusMethodNotAllowed {
				t.Errorf("%s /api/portal/info: status=%d, want 405", method, resp.StatusCode)
			}
		})
	}
}

// TestGetPortalInfo_OptionsPreflight documents the current CORS-preflight
// behaviour at /api/portal/info.
//
// The portal binary does NOT install any CORS middleware on this route
// (search `cmd/portal/main.go` for `Access-Control` — no hits at the time
// of writing). chi's default router responds to OPTIONS for a registered
// path with the auto-allowed-methods header but no `Access-Control-*`
// headers, which means cross-origin browser requests would fail preflight
// in dev unless the operator runs the SPA from the same origin.
//
// Rather than assert a specific CORS contract that doesn't exist, this
// test pins what IS true today: OPTIONS receives a non-5xx response and
// no `Access-Control-Allow-Origin` is present. If a future change adds
// CORS support to the public bootstrap endpoint, the test should be
// flipped to assert the new headers.
//
// Story: gate-tests-portalinfo-method-not-allowed-cors
func TestGetPortalInfo_OptionsPreflight(t *testing.T) {
	h := &portalinfo.Handler{PlaygroundEnabled: true, LandingVariant: "auto"}
	env := newTestEnv(t, h)

	req, err := http.NewRequest(http.MethodOptions, env.srv.URL+"/api/portal/info", nil)
	if err != nil {
		t.Fatalf("new request: %v", err)
	}
	// Browser-style preflight headers — even though no CORS middleware is
	// installed, this is the shape the test pins against.
	req.Header.Set("Origin", "https://example.com")
	req.Header.Set("Access-Control-Request-Method", "GET")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("OPTIONS /api/portal/info: %v", err)
	}
	defer resp.Body.Close()

	// The endpoint must respond — not 5xx, not connection-reset. The
	// specific status (200, 204, 405) depends on chi's routing; pin only
	// the "doesn't blow up" contract.
	if resp.StatusCode >= 500 {
		t.Errorf("OPTIONS /api/portal/info: status=%d, want <500", resp.StatusCode)
	}

	// Current behaviour: NO Access-Control-Allow-Origin header. If this
	// flips, the test must be updated to assert the new CORS contract;
	// surfacing the change here prevents silent CORS regressions on the
	// public bootstrap surface.
	if got := resp.Header.Get("Access-Control-Allow-Origin"); got != "" {
		t.Errorf("Access-Control-Allow-Origin: got %q, want empty (no CORS middleware on /api/portal/info today — update this test if CORS support is added intentionally)", got)
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
