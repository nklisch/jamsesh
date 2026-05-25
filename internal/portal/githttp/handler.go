// Package githttp implements the git smart-HTTP handler for jamsesh portal
// session repositories. It exposes the three smart-HTTP endpoints
// (info/refs, git-upload-pack, git-receive-pack) behind an HTTP Basic auth
// + session-membership middleware chain.
package githttp

import (
	"context"
	"fmt"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/portal/postreceive"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/tokens"
)

// playgroundOrgID is the hard-coded org_id for the reserved playground org.
// Defined locally to avoid an import cycle (githttp → playground would be
// cyclic). Value must match playground.ReservedOrgID.
const playgroundOrgID = "org_playground"

// lifecycleAcquirer is the subset of objectstore.LifecycleManager used by the
// git smart-HTTP handler. Defined locally so tests can inject a stub without
// depending on the full objectstore package.
type lifecycleAcquirer interface {
	AcquireForRequest(ctx context.Context, sessionID string) (lease.Handle, error)
}

// githttpStore is the minimal store interface consumed by Handler.
type githttpStore interface {
	store.SessionStore
	store.SessionMemberStore
	store.PlaygroundSessionStore
}

// Handler is the git smart-HTTP handler. Construct with all fields set and
// call Mount to register routes on a chi router.
type Handler struct {
	Store     githttpStore
	Tokens    tokens.Service
	Storage   storage.Service
	Validator *prereceive.Validator
	Emitter   *postreceive.Emitter
	// Lifecycle, when non-nil, hydrates the bare repo on first request for a
	// session on this pod and holds the distributed lease. Nil in single-instance
	// mode — the bare repo lives on the only pod's local disk from session-create
	// onward. Mirrors postreceive.Emitter's Lifecycle field shape and semantics.
	Lifecycle lifecycleAcquirer
	// Metrics is optional; when non-nil, git push outcomes increment
	// GitPushesTotal with result labels "ok" or "rejected".
	Metrics *metrics.Registry
	// ReceivePackSem is a counting semaphore that limits concurrent
	// git-receive-pack handlers. When full, new requests are rejected with
	// 503 Retry-After. If nil, no concurrency limit is enforced.
	// Initialise with make(chan struct{}, N) where N is the desired cap.
	ReceivePackSem chan struct{}
	// PlaygroundIdleTimeout is the idle-timeout duration for playground sessions.
	// When a commit push to a playground session succeeds, the session's
	// last_substantive_activity_at and idle_timeout_at are reset using this
	// value (idle_timeout_at = now + PlaygroundIdleTimeout). A zero value
	// disables the activity-reset call — the session's destruction sweep will
	// still work but will use the original idle_timeout_at.
	PlaygroundIdleTimeout time.Duration
	// Clock is the injectable time source used for playground activity-reset
	// timestamps. When nil, RealClock() is used automatically. Tests inject a
	// deterministic clock so the reset timestamps are reproducible.
	Clock Clock
}

// acquireForGitRequest invokes LifecycleManager.AcquireForRequest in
// clustered mode to ensure the session's bare repo is hydrated to this pod's
// local cache before serving the smart-HTTP operation. The returned handle is
// owned by LifecycleManager (idle eviction / LRU / lease loss / SIGTERM); the
// caller MUST NOT release it. Returns nil immediately when Lifecycle is nil
// (single-instance mode).
func (h *Handler) acquireForGitRequest(ctx context.Context, sessionID string) error {
	if h.Lifecycle == nil {
		return nil
	}
	_, err := h.Lifecycle.AcquireForRequest(ctx, sessionID)
	if err != nil {
		return fmt.Errorf("githttp: hydrate session %s: %w", sessionID, err)
	}
	return nil
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
