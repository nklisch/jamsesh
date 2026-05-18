package router_test

// body_limits_api_test.go — real-endpoint body-size cap tests.
//
// These tests verify that the BodyLimit middleware (mounted at /api/* by
// router.New) interacts correctly with the oapi-codegen strict handler's
// JSON decode path:
//
//   request body > limit
//     → MaxBytesReader sets error on ResponseWriter
//     → json.NewDecoder.Decode returns *http.MaxBytesError
//     → strict handler calls RequestErrorHandlerFunc (httperr.WriteBadRequest)
//     → WriteBadRequest detects *http.MaxBytesError → ErrBodyTooLarge()
//     → 413 {"error":"request.body_too_large"}
//
// Each test case fires a POST at a real API path — using the generated
// strict handler wired with httperr.WriteBadRequest — so any handler that
// bypasses the strict decode layer would surface here.
//
// See also: body_limits_test.go for the lower-level middleware unit tests.

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/router"
)

// ---------------------------------------------------------------------------
// Stub StrictServerInterface
//
// stubStrict satisfies openapi.StrictServerInterface with no-op methods.
// These methods are never reached in the over-limit tests — the strict
// handler's JSON decoder fires RequestErrorHandlerFunc before calling the
// business handler.
// ---------------------------------------------------------------------------

type stubStrict struct{}

func (stubStrict) ExchangeMagicLink(_ context.Context, _ openapi.ExchangeMagicLinkRequestObject) (openapi.ExchangeMagicLinkResponseObject, error) {
	return nil, nil
}
func (stubStrict) RequestMagicLink(_ context.Context, _ openapi.RequestMagicLinkRequestObject) (openapi.RequestMagicLinkResponseObject, error) {
	return nil, nil
}
func (stubStrict) OauthCallback(_ context.Context, _ openapi.OauthCallbackRequestObject) (openapi.OauthCallbackResponseObject, error) {
	return nil, nil
}
func (stubStrict) StartOAuth(_ context.Context, _ openapi.StartOAuthRequestObject) (openapi.StartOAuthResponseObject, error) {
	return nil, nil
}
func (stubStrict) RefreshToken(_ context.Context, _ openapi.RefreshTokenRequestObject) (openapi.RefreshTokenResponseObject, error) {
	return nil, nil
}
func (stubStrict) RevokeToken(_ context.Context, _ openapi.RevokeTokenRequestObject) (openapi.RevokeTokenResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetMe(_ context.Context, _ openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	return nil, nil
}
func (stubStrict) CreateOrg(_ context.Context, _ openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetOrg(_ context.Context, _ openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	return nil, nil
}
func (stubStrict) PatchOrg(_ context.Context, _ openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
	return nil, nil
}
func (stubStrict) CreateOrgInvite(_ context.Context, _ openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	return nil, nil
}
func (stubStrict) AcceptOrgInvite(_ context.Context, _ openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	return nil, nil
}
func (stubStrict) ListOrgMembers(_ context.Context, _ openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	return nil, nil
}
func (stubStrict) ListSessions(_ context.Context, _ openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	return nil, nil
}
func (stubStrict) CreateSession(_ context.Context, _ openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetSession(_ context.Context, _ openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	return nil, nil
}
func (stubStrict) PatchSession(_ context.Context, _ openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	return nil, nil
}
func (stubStrict) AbandonSession(_ context.Context, _ openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	return nil, nil
}
func (stubStrict) ListComments(_ context.Context, _ openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	return nil, nil
}
func (stubStrict) CreateComment(_ context.Context, _ openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	return nil, nil
}
func (stubStrict) ResolveComment(_ context.Context, _ openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetSessionDigest(_ context.Context, _ openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetSessionFile(_ context.Context, _ openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	return nil, nil
}
func (stubStrict) FinalizeSession(_ context.Context, _ openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetFinalizePlan(_ context.Context, _ openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	return nil, nil
}
func (stubStrict) IssueFetchToken(_ context.Context, _ openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	return nil, nil
}
func (stubStrict) AcquireFinalizeLock(_ context.Context, _ openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	return nil, nil
}
func (stubStrict) ReleaseFinalizeLock(_ context.Context, _ openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	return nil, nil
}
func (stubStrict) PatchFinalizeLock(_ context.Context, _ openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	return nil, nil
}
func (stubStrict) InviteToSession(_ context.Context, _ openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	return nil, nil
}
func (stubStrict) GetSessionInvite(_ context.Context, _ openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	return nil, nil
}
func (stubStrict) AcceptSessionInvite(_ context.Context, _ openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	return nil, nil
}
func (stubStrict) MarkSessionShipped(_ context.Context, _ openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	return nil, nil
}
func (stubStrict) RemoveSessionMember(_ context.Context, _ openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	return nil, nil
}
func (stubStrict) UpsertRefMode(_ context.Context, _ openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	return nil, nil
}
func (stubStrict) ListSessionRefs(_ context.Context, _ openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	return nil, nil
}
func (stubStrict) IssueWsTicket(_ context.Context, _ openapi.IssueWsTicketRequestObject) (openapi.IssueWsTicketResponseObject, error) {
	return nil, nil
}

// ---------------------------------------------------------------------------
// Helper: build a test router wired with the real strict handler pipeline.
//
// mountStrictRoutes registers only the POST routes under test so that the
// strict handler's decode path is exercised, with httperr.WriteBadRequest as
// RequestErrorHandlerFunc (the same wiring used in production).
// ---------------------------------------------------------------------------

func newStrictAPIRouter(apiBodyLimitBytes int64) http.Handler {
	si := openapi.NewStrictHandlerWithOptions(stubStrict{}, nil, openapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  httperr.WriteBadRequest,
		ResponseErrorHandlerFunc: httperr.WriteFromError,
	})
	wrapper := &openapi.ServerInterfaceWrapper{
		Handler:          si,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	return router.New(router.Deps{
		APIBodyLimitBytes: apiBodyLimitBytes,
		MountAPI: func(r chi.Router) {
			// Register only the POST endpoints under test. Path segments are
			// relative to the /api group that router.New already creates.
			r.Post("/auth/magic-link/request", wrapper.RequestMagicLink)
			r.Post("/orgs/{orgID}/sessions", wrapper.CreateSession)
			r.Post("/orgs/{orgID}/sessions/{sessionID}/comments", wrapper.CreateComment)
			r.Post("/orgs/{orgID}/invites", wrapper.CreateOrgInvite)
			r.Patch("/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}", wrapper.PatchFinalizeLock)
		},
	})
}

// ---------------------------------------------------------------------------
// TestREST_BodySizeCap_Returns413_TableDriven
//
// Fires a 2 MiB POST body at each real endpoint and asserts:
//   - HTTP 413
//   - JSON envelope {"error":"request.body_too_large"}
//
// The stub handler is never called — the strict decode layer fires
// RequestErrorHandlerFunc (httperr.WriteBadRequest) which detects
// *http.MaxBytesError and emits ErrBodyTooLarge().
// ---------------------------------------------------------------------------

func TestREST_BodySizeCap_Returns413_TableDriven(t *testing.T) {
	t.Parallel()

	h := newStrictAPIRouter(0) // 0 → default 1 MiB

	// overBody is 2 MiB of valid JSON — a large string value that the JSON
	// decoder would accept if it got that far. The key "x" and surrounding
	// braces are 6 bytes; the value fills the rest to push past 2 MiB.
	const bodySize = 2 << 20 // 2 MiB
	filler := strings.Repeat("a", bodySize-6)
	overBody := `{"x":"` + filler + `"}`

	cases := []struct {
		name   string
		method string
		path   string
	}{
		{
			name:   "RequestMagicLink",
			method: http.MethodPost,
			path:   "/api/auth/magic-link/request",
		},
		{
			name:   "CreateSession",
			method: http.MethodPost,
			path:   "/api/orgs/org-01/sessions",
		},
		{
			name:   "CreateComment",
			method: http.MethodPost,
			path:   "/api/orgs/org-01/sessions/sess-01/comments",
		},
		{
			name:   "CreateOrgInvite",
			method: http.MethodPost,
			path:   "/api/orgs/org-01/invites",
		},
		{
			name:   "PatchFinalizeLock",
			method: http.MethodPatch,
			path:   "/api/orgs/org-01/sessions/sess-01/finalize/lock/lock-01",
		},
	}

	for _, tc := range cases {
		tc := tc
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()

			w := httptest.NewRecorder()
			req := httptest.NewRequest(tc.method, tc.path, strings.NewReader(overBody))
			req.Header.Set("Content-Type", "application/json")
			h.ServeHTTP(w, req)

			if w.Code != http.StatusRequestEntityTooLarge {
				t.Errorf("%s: want 413, got %d\nbody: %s", tc.name, w.Code, w.Body.String())
				return
			}

			var env struct {
				Error string `json:"error"`
			}
			if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
				t.Fatalf("%s: decode response envelope: %v", tc.name, err)
			}
			if env.Error != "request.body_too_large" {
				t.Errorf("%s: want error=request.body_too_large, got %q", tc.name, env.Error)
			}
		})
	}
}

// ---------------------------------------------------------------------------
// TestREST_BodySizeCap_HonorsConfig
//
// Builds a router with APIBodyLimitBytes=256 and sends a 512-byte body.
// The cap is configurable; this test verifies the Deps field is honoured.
// ---------------------------------------------------------------------------

func TestREST_BodySizeCap_HonorsConfig(t *testing.T) {
	t.Parallel()

	const cap int64 = 256
	h := newStrictAPIRouter(cap)

	// 512 bytes of JSON — over the 256-byte cap but well under the default 1 MiB.
	body := `{"email":"` + strings.Repeat("a", 502) + `@x.com"}`

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost, "/api/auth/magic-link/request", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	h.ServeHTTP(w, req)

	if w.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("want 413 with 256-byte cap and 512-byte body, got %d\nbody: %s", w.Code, w.Body.String())
		return
	}

	var env struct {
		Error string `json:"error"`
	}
	if err := json.NewDecoder(w.Body).Decode(&env); err != nil {
		t.Fatalf("decode response envelope: %v", err)
	}
	if env.Error != "request.body_too_large" {
		t.Errorf("want error=request.body_too_large, got %q", env.Error)
	}
}

// ---------------------------------------------------------------------------
// TestREST_BodySizeCap_GitSmartHTTPUnaffected
//
// Confirms that /git/* routes are NOT subject to the 1 MiB API cap.
// A stub git handler receives the oversized body without a 413.
// If the handler were under the API BodyLimit, it would receive 413 before
// the stub could run; the test asserts the stub was reached (200).
// ---------------------------------------------------------------------------

func TestREST_BodySizeCap_GitSmartHTTPUnaffected(t *testing.T) {
	t.Parallel()

	var gitHandlerCalled bool

	h := router.New(router.Deps{
		MountGit: func(r chi.Router) {
			r.Post("/{org}/{repo}.git/git-receive-pack", func(w http.ResponseWriter, r *http.Request) {
				// Drain body to simulate git-receive-pack reading pack data.
				buf := make([]byte, 32*1024)
				total := 0
				for {
					n, err := r.Body.Read(buf)
					total += n
					if err != nil {
						break
					}
				}
				gitHandlerCalled = true
				w.WriteHeader(http.StatusOK)
			})
		},
	})

	// 2 MiB body — over the 1 MiB API cap but should reach the git handler.
	overBody := bytes.Repeat([]byte("x"), 2<<20)

	w := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPost,
		"/git/myorg/myrepo.git/git-receive-pack",
		bytes.NewReader(overBody))
	req.Header.Set("Content-Type", "application/x-git-receive-pack-request")
	h.ServeHTTP(w, req)

	if !gitHandlerCalled {
		t.Error("git handler was not called — /git/* route may be under the API BodyLimit")
	}
	if w.Code == http.StatusRequestEntityTooLarge {
		t.Error("got 413 on /git/* route — BodyLimit is incorrectly applied to git routes")
	}
}
