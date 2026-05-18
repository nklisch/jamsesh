// Package githttp implements the git smart-HTTP handler for jamsesh portal
// session repositories. It exposes the three smart-HTTP endpoints
// (info/refs, git-upload-pack, git-receive-pack) behind an HTTP Basic auth
// + session-membership middleware chain.
package githttp

import (
	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/metrics"
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
	// Metrics is optional; when non-nil, git push outcomes increment
	// GitPushesTotal with result labels "ok" or "rejected".
	Metrics *metrics.Registry
	// ReceivePackSem is a counting semaphore that limits concurrent
	// git-receive-pack handlers. When full, new requests are rejected with
	// 503 Retry-After. If nil, no concurrency limit is enforced.
	// Initialise with make(chan struct{}, N) where N is the desired cap.
	ReceivePackSem chan struct{}
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
		r.Get("/info/refs", h.infoRefs)
		r.Post("/git-upload-pack", h.uploadPack)
		r.Post("/git-receive-pack", h.receivePack)
	})
}
