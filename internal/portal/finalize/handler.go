package finalize

import (
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
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

// Handler implements the openapi.StrictServerInterface methods that the
// finalize feature owns. Each endpoint lives in its own file in this
// package; this file defines the struct + constructor.
//
// Plan / fetch-token / mark-shipped land in stories 2 and 3.
type Handler struct {
	store     store.Store
	storage   storage.Service
	events    *events.Log
	tokens    tokens.Service
	portalURL string
	clock     Clock
}

// New constructs a Handler with the real system clock. portalURL is the
// public origin used to compose the git smart-HTTP fallback URL in plan
// responses (story 2/3).
func New(s store.Store, stor storage.Service, log *events.Log, tok tokens.Service, portalURL string) *Handler {
	return NewWithClock(s, stor, log, tok, portalURL, realClock{})
}

// NewWithClock constructs a Handler with the supplied clock. Used by unit
// tests (fakeClock) and the e2etest-tagged binary (testclock.AdvanceableClock).
func NewWithClock(s store.Store, stor storage.Service, log *events.Log, tok tokens.Service, portalURL string, clock Clock) *Handler {
	return &Handler{
		store:     s,
		storage:   stor,
		events:    log,
		tokens:    tok,
		portalURL: portalURL,
		clock:     clock,
	}
}
