package sessions_test

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/sessions"
	"jamsesh/internal/portal/tokens"
)

// handlerFakeClock is a controllable time source used to exercise the
// clock-injection path on the sessions.Handler. Mirrors the shape of
// handlerFakeClock in internal/portal/finalize/clock_test.go.
type handlerFakeClock struct {
	t time.Time
}

func (f *handlerFakeClock) Now() time.Time { return f.t }

// TestHandler_CreateSessionUsesInjectedClock asserts that the CreatedAt
// stamp written by CreateSession reflects the clock supplied to
// NewWithClock — i.e. the injected clock fully replaced the real time
// source. We rebuild a one-off HTTP server backed by NewWithClock so
// the bearer middleware and strict-handler wiring are identical to
// production.
func TestHandler_CreateSessionUsesInjectedClock(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator-clock@example.com")
	org := seedOrg(t, env.s, "Clock Test Org", "clock-test-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	fixed := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	clk := &handlerFakeClock{t: fixed}

	srv := newClockSessionsSrv(t, env, clk)
	token := env.bearerToken(t, acc.ID)

	body := map[string]any{
		"name":         "Clock injection test",
		"goal":         "Verify clock injection",
		"scope":        `["src/**"]`,
		"default_mode": "sync",
	}
	resp := postJSON(t, srv, "/api/orgs/"+org.ID+"/sessions", token, body)
	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var sess openapi.Session
	decodeBody(t, resp, &sess)

	if !sess.CreatedAt.Equal(fixed) {
		t.Errorf("CreatedAt: want %v, got %v", fixed, sess.CreatedAt)
	}
}

// TestHandler_NewVsNewWithClock_ProductionPathClean asserts that the
// default New() constructor produces a Handler whose CreateSession
// still works (the realClock path). Regression check that the
// constructor refactor didn't break the default path: a session
// created via the standard env should have a CreatedAt stamp in
// [before, after].
func TestHandler_NewVsNewWithClock_ProductionPathClean(t *testing.T) {
	env := newTestEnv(t)
	acc := seedAccount(t, env.s, "creator-realclock@example.com")
	org := seedOrg(t, env.s, "Realclock Org", "realclock-org")
	seedOrgMember(t, env.s, org.ID, acc.ID, "creator")

	token := env.bearerToken(t, acc.ID)
	body := map[string]any{
		"name":         "RealClock Path",
		"goal":         "verify default constructor",
		"scope":        `["**"]`,
		"default_mode": "sync",
	}

	before := time.Now().UTC()
	resp := postJSON(t, env.srv, "/api/orgs/"+org.ID+"/sessions", token, body)
	after := time.Now().UTC()

	if resp.StatusCode != http.StatusCreated {
		t.Fatalf("expected 201, got %d", resp.StatusCode)
	}
	var sess openapi.Session
	decodeBody(t, resp, &sess)

	// The realClock should produce a stamp inside [before, after].
	if sess.CreatedAt.Before(before) || sess.CreatedAt.After(after) {
		t.Errorf("CreatedAt %v not in [%v, %v]", sess.CreatedAt, before, after)
	}
}

// newClockSessionsSrv mounts a one-off httptest.Server whose sessions
// handler is built via NewWithClock(clk). Mirrors the shape of
// newTestEnvWithStore in handler_test.go but only registers the
// CreateSession route since that's all the injected-clock test
// exercises.
func newClockSessionsSrv(t *testing.T, env *testEnv, clk sessions.Clock) *httptest.Server {
	t.Helper()
	h := sessions.NewWithClock(env.s, env.stor, env.eventLog, env.sender, "http://localhost:8443", clk)
	strictAPI := openapi.NewStrictHandlerWithOptions(&sessionsOnlyStrict{h}, nil,
		openapi.StrictHTTPServerOptions{
			RequestErrorHandlerFunc:  httperr.WriteBadRequest,
			ResponseErrorHandlerFunc: httperr.WriteFromError,
		})
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictAPI,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}
	r := chi.NewRouter()
	r.Group(func(r chi.Router) {
		r.Use(tokens.BearerMiddleware(env.svc))
		r.Post("/api/orgs/{orgID}/sessions", apiWrapper.CreateSession)
	})
	srv := httptest.NewServer(r)
	t.Cleanup(srv.Close)
	return srv
}
