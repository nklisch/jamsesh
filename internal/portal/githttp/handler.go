// Package githttp implements the git smart-HTTP handler for jamsesh portal
// session repositories. It exposes the three smart-HTTP endpoints
// (info/refs, git-upload-pack, git-receive-pack) behind an HTTP Basic auth
// + session-membership middleware chain.
package githttp

import (
	"net/http"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/postreceive"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// Handler is the git smart-HTTP handler. Construct with all fields set and
// call Mount to register routes on a chi router.
type Handler struct {
	Store     store.Store
	Tokens    tokens.Service
	Storage   storage.Service
	Validator *prereceive.Validator
	Emitter   *postreceive.Emitter
}

// Mount registers the smart-HTTP routes relative to the base path the caller
// has already established. The router.Deps.MountGit hook is invoked under
// "/git", so routes here are relative to that prefix:
//
//	GET  /{orgID}/{sessionID}.git/info/refs
//	POST /{orgID}/{sessionID}.git/git-upload-pack
//	POST /{orgID}/{sessionID}.git/git-receive-pack
func (h *Handler) Mount(r chi.Router) {
	r.Route("/{orgID}/{sessionID}.git", func(r chi.Router) {
		r.Use(h.basicAuth, h.requireSessionMember, h.checkArchived)
		r.Get("/info/refs", h.stubInfoRefs)
		r.Post("/git-upload-pack", h.stubUploadPack)
		r.Post("/git-receive-pack", h.stubReceivePack)
	})
}

// stubInfoRefs is the placeholder for the info/refs endpoint.
// Replaced by the upload-pack-fetch story.
func (h *Handler) stubInfoRefs(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// stubUploadPack is the placeholder for the git-upload-pack endpoint.
// Replaced by the upload-pack-fetch story.
func (h *Handler) stubUploadPack(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}

// stubReceivePack is the placeholder for the git-receive-pack endpoint.
// Replaced by the receive-pack-push story.
func (h *Handler) stubReceivePack(w http.ResponseWriter, _ *http.Request) {
	http.Error(w, "not implemented", http.StatusNotImplemented)
}
