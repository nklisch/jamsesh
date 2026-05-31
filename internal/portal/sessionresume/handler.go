package sessionresume

import (
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/tokens"
)

// Clock is an injectable time source. Mirrors auth.Clock and tokens.Clock so a
// single *testclock.AdvanceableClock satisfies all of them. Per-package types
// avoid cross-package import coupling — structural typing carries the
// "advance once, move everywhere" property.
type Clock interface {
	Now() time.Time
}

type realClock struct{}

func (realClock) Now() time.Time { return time.Now().UTC() }

// sessionResumeStore is the minimal store interface consumed by Handler and the
// package-private helper functions (checkSessionMembership, ExchangeSessionResume).
type sessionResumeStore interface {
	store.SessionStore
	store.SessionMemberStore
	store.OrgMemberStore
	store.ResumeTokenStore
	store.AccountStore
}

// Handler implements the openapi.StrictServerInterface methods that the
// session-resume feature owns. The mint endpoint lives in mint.go.
type Handler struct {
	store     sessionResumeStore
	tokens    tokens.Service
	portalURL string
	clock     Clock
}

// New constructs a Handler with the real system clock.
func New(s sessionResumeStore, tok tokens.Service, portalURL string) *Handler {
	return NewWithClock(s, tok, portalURL, realClock{})
}

// NewWithClock constructs a Handler with the supplied clock. Used by unit tests
// and e2etest builds (testclock.AdvanceableClock).
func NewWithClock(s sessionResumeStore, tok tokens.Service, portalURL string, clock Clock) *Handler {
	return &Handler{
		store:     s,
		tokens:    tok,
		portalURL: portalURL,
		clock:     clock,
	}
}
