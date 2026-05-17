package finalize

import (
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

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
}

// New constructs a Handler. portalURL is the public origin used to compose
// the git smart-HTTP fallback URL in plan responses (story 2/3).
func New(s store.Store, stor storage.Service, log *events.Log, tok tokens.Service, portalURL string) *Handler {
	return &Handler{
		store:     s,
		storage:   stor,
		events:    log,
		tokens:    tok,
		portalURL: portalURL,
	}
}
