// Command portal is the jamsesh portal server binary.
//
// Usage:
//
//	portal [--config path/to/config.yaml]
//
// All configuration fields can be overridden with environment variables;
// see package config for the full list. Configuration is loaded from
// the optional YAML file first, then env vars overlay it.
//
// The binary serves only /healthz until sibling features mount API,
// Git, MCP, and WebSocket handlers into router.Deps. Add mount hooks
// here as they land.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log/slog"
	"os"
	"os/signal"
	"sync"
	"syscall"
	"time"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/portal/accounts"
	"jamsesh/internal/portal/assets"
	"jamsesh/internal/portal/auth"
	"jamsesh/internal/portal/automerger"
	"jamsesh/internal/portal/comments"
	"jamsesh/internal/portal/config"
	"jamsesh/internal/portal/events"
	"jamsesh/internal/portal/finalize"
	"jamsesh/internal/portal/githttp"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/lease"
	"jamsesh/internal/portal/logging"
	"jamsesh/internal/portal/mcpendpoint"
	"jamsesh/internal/portal/metrics"
	portaloauth "jamsesh/internal/portal/oauth"
	"jamsesh/internal/portal/playground"
	"jamsesh/internal/portal/portalinfo"
	"jamsesh/internal/portal/postreceive"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/probes"
	"jamsesh/internal/portal/router"
	"jamsesh/internal/portal/ratelimit"
	"jamsesh/internal/portal/senders"
	"jamsesh/internal/portal/sessionresume"
	"jamsesh/internal/portal/server"
	"jamsesh/internal/portal/sessions"
	"jamsesh/internal/portal/storage"
	"jamsesh/internal/portal/storage/objectstore"
	"jamsesh/internal/portal/tokens"
	"jamsesh/internal/portal/wsgateway"
)

// combinedHandler satisfies openapi.StrictServerInterface by composing the
// individual feature handlers. Each feature handler owns its own methods;
// this type is purely a wiring shim — no business logic lives here.
type combinedHandler struct {
	*tokens.Handler
	*auth.MagicLinkHandler
	*auth.OAuthHandler
	AccountsHandler   *accounts.Handler
	SessionsHandler   *sessions.Handler
	CommentsHandler   *comments.Handler
	FinalizeHandler       *finalize.Handler
	SessionResumeHandler  *sessionresume.Handler
	WsTicketHandler       *wsgateway.WsTicketHandler
	PlaygroundHandler *playground.Handler
	PortalInfoHandler *portalinfo.Handler
}

// GetMe delegates to the accounts handler.
func (c *combinedHandler) GetMe(ctx context.Context, req openapi.GetMeRequestObject) (openapi.GetMeResponseObject, error) {
	return c.AccountsHandler.GetMe(ctx, req)
}

// CreateOrg delegates to the accounts handler.
func (c *combinedHandler) CreateOrg(ctx context.Context, req openapi.CreateOrgRequestObject) (openapi.CreateOrgResponseObject, error) {
	return c.AccountsHandler.CreateOrg(ctx, req)
}

// ListOrgMembers delegates to the accounts handler.
func (c *combinedHandler) ListOrgMembers(ctx context.Context, req openapi.ListOrgMembersRequestObject) (openapi.ListOrgMembersResponseObject, error) {
	return c.AccountsHandler.ListOrgMembers(ctx, req)
}

// CreateOrgInvite delegates to the accounts handler.
func (c *combinedHandler) CreateOrgInvite(ctx context.Context, req openapi.CreateOrgInviteRequestObject) (openapi.CreateOrgInviteResponseObject, error) {
	return c.AccountsHandler.CreateOrgInvite(ctx, req)
}

// AcceptOrgInvite delegates to the accounts handler.
func (c *combinedHandler) AcceptOrgInvite(ctx context.Context, req openapi.AcceptOrgInviteRequestObject) (openapi.AcceptOrgInviteResponseObject, error) {
	return c.AccountsHandler.AcceptOrgInvite(ctx, req)
}

// GetOrg delegates to the accounts handler.
func (c *combinedHandler) GetOrg(ctx context.Context, req openapi.GetOrgRequestObject) (openapi.GetOrgResponseObject, error) {
	return c.AccountsHandler.GetOrg(ctx, req)
}

// PatchOrg delegates to the accounts handler.
func (c *combinedHandler) PatchOrg(ctx context.Context, req openapi.PatchOrgRequestObject) (openapi.PatchOrgResponseObject, error) {
	return c.AccountsHandler.PatchOrg(ctx, req)
}

// CreateSession delegates to the sessions handler.
func (c *combinedHandler) CreateSession(ctx context.Context, req openapi.CreateSessionRequestObject) (openapi.CreateSessionResponseObject, error) {
	return c.SessionsHandler.CreateSession(ctx, req)
}

// PatchSession delegates to the sessions handler.
func (c *combinedHandler) PatchSession(ctx context.Context, req openapi.PatchSessionRequestObject) (openapi.PatchSessionResponseObject, error) {
	return c.SessionsHandler.PatchSession(ctx, req)
}

// FinalizeSession delegates to the sessions handler.
func (c *combinedHandler) FinalizeSession(ctx context.Context, req openapi.FinalizeSessionRequestObject) (openapi.FinalizeSessionResponseObject, error) {
	return c.SessionsHandler.FinalizeSession(ctx, req)
}

// AbandonSession delegates to the sessions handler.
func (c *combinedHandler) AbandonSession(ctx context.Context, req openapi.AbandonSessionRequestObject) (openapi.AbandonSessionResponseObject, error) {
	return c.SessionsHandler.AbandonSession(ctx, req)
}

// ListSessions delegates to the sessions handler.
func (c *combinedHandler) ListSessions(ctx context.Context, req openapi.ListSessionsRequestObject) (openapi.ListSessionsResponseObject, error) {
	return c.SessionsHandler.ListSessions(ctx, req)
}

// GetSession delegates to the sessions handler.
func (c *combinedHandler) GetSession(ctx context.Context, req openapi.GetSessionRequestObject) (openapi.GetSessionResponseObject, error) {
	return c.SessionsHandler.GetSession(ctx, req)
}

// ListSessionRefs delegates to the sessions handler.
func (c *combinedHandler) ListSessionRefs(ctx context.Context, req openapi.ListSessionRefsRequestObject) (openapi.ListSessionRefsResponseObject, error) {
	return c.SessionsHandler.ListSessionRefs(ctx, req)
}

// GetSessionDigest delegates to the sessions handler.
func (c *combinedHandler) GetSessionDigest(ctx context.Context, req openapi.GetSessionDigestRequestObject) (openapi.GetSessionDigestResponseObject, error) {
	return c.SessionsHandler.GetSessionDigest(ctx, req)
}

// InviteToSession delegates to the sessions handler.
func (c *combinedHandler) InviteToSession(ctx context.Context, req openapi.InviteToSessionRequestObject) (openapi.InviteToSessionResponseObject, error) {
	return c.SessionsHandler.InviteToSession(ctx, req)
}

// GetSessionInvite delegates to the sessions handler.
func (c *combinedHandler) GetSessionInvite(ctx context.Context, req openapi.GetSessionInviteRequestObject) (openapi.GetSessionInviteResponseObject, error) {
	return c.SessionsHandler.GetSessionInvite(ctx, req)
}

// AcceptSessionInvite delegates to the sessions handler.
func (c *combinedHandler) AcceptSessionInvite(ctx context.Context, req openapi.AcceptSessionInviteRequestObject) (openapi.AcceptSessionInviteResponseObject, error) {
	return c.SessionsHandler.AcceptSessionInvite(ctx, req)
}

// RemoveSessionMember delegates to the sessions handler.
func (c *combinedHandler) RemoveSessionMember(ctx context.Context, req openapi.RemoveSessionMemberRequestObject) (openapi.RemoveSessionMemberResponseObject, error) {
	return c.SessionsHandler.RemoveSessionMember(ctx, req)
}

// ListComments delegates to the comments handler.
func (c *combinedHandler) ListComments(ctx context.Context, req openapi.ListCommentsRequestObject) (openapi.ListCommentsResponseObject, error) {
	return c.CommentsHandler.ListComments(ctx, req)
}

// CreateComment delegates to the comments handler.
func (c *combinedHandler) CreateComment(ctx context.Context, req openapi.CreateCommentRequestObject) (openapi.CreateCommentResponseObject, error) {
	return c.CommentsHandler.CreateComment(ctx, req)
}

// ResolveComment delegates to the comments handler.
func (c *combinedHandler) ResolveComment(ctx context.Context, req openapi.ResolveCommentRequestObject) (openapi.ResolveCommentResponseObject, error) {
	return c.CommentsHandler.ResolveComment(ctx, req)
}

// GetSessionFile delegates to the sessions handler.
func (c *combinedHandler) GetSessionFile(ctx context.Context, req openapi.GetSessionFileRequestObject) (openapi.GetSessionFileResponseObject, error) {
	return c.SessionsHandler.GetSessionFile(ctx, req)
}

// UpsertRefMode delegates to the sessions handler.
func (c *combinedHandler) UpsertRefMode(ctx context.Context, req openapi.UpsertRefModeRequestObject) (openapi.UpsertRefModeResponseObject, error) {
	return c.SessionsHandler.UpsertRefMode(ctx, req)
}

// AcquireFinalizeLock delegates to the finalize handler.
func (c *combinedHandler) AcquireFinalizeLock(ctx context.Context, req openapi.AcquireFinalizeLockRequestObject) (openapi.AcquireFinalizeLockResponseObject, error) {
	return c.FinalizeHandler.AcquireFinalizeLock(ctx, req)
}

// PatchFinalizeLock delegates to the finalize handler.
func (c *combinedHandler) PatchFinalizeLock(ctx context.Context, req openapi.PatchFinalizeLockRequestObject) (openapi.PatchFinalizeLockResponseObject, error) {
	return c.FinalizeHandler.PatchFinalizeLock(ctx, req)
}

// ReleaseFinalizeLock delegates to the finalize handler.
func (c *combinedHandler) ReleaseFinalizeLock(ctx context.Context, req openapi.ReleaseFinalizeLockRequestObject) (openapi.ReleaseFinalizeLockResponseObject, error) {
	return c.FinalizeHandler.ReleaseFinalizeLock(ctx, req)
}

// GetFinalizePlan delegates to the finalize handler.
func (c *combinedHandler) GetFinalizePlan(ctx context.Context, req openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	return c.FinalizeHandler.GetFinalizePlan(ctx, req)
}

// IssueFetchToken delegates to the finalize handler.
func (c *combinedHandler) IssueFetchToken(ctx context.Context, req openapi.IssueFetchTokenRequestObject) (openapi.IssueFetchTokenResponseObject, error) {
	return c.FinalizeHandler.IssueFetchToken(ctx, req)
}

// CreateSessionResume delegates to the session-resume handler.
func (c *combinedHandler) CreateSessionResume(ctx context.Context, req openapi.CreateSessionResumeRequestObject) (openapi.CreateSessionResumeResponseObject, error) {
	return c.SessionResumeHandler.CreateSessionResume(ctx, req)
}

// ExchangeSessionResume delegates to the session-resume handler.
func (c *combinedHandler) ExchangeSessionResume(ctx context.Context, req openapi.ExchangeSessionResumeRequestObject) (openapi.ExchangeSessionResumeResponseObject, error) {
	return c.SessionResumeHandler.ExchangeSessionResume(ctx, req)
}

// MarkSessionShipped delegates to the finalize handler.
func (c *combinedHandler) MarkSessionShipped(ctx context.Context, req openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	return c.FinalizeHandler.MarkSessionShipped(ctx, req)
}

// IssueWsTicket delegates to the ws-ticket handler.
func (c *combinedHandler) IssueWsTicket(ctx context.Context, req openapi.IssueWsTicketRequestObject) (openapi.IssueWsTicketResponseObject, error) {
	return c.WsTicketHandler.IssueWsTicket(ctx, req)
}

// CreatePlaygroundSession delegates to the playground handler.
func (c *combinedHandler) CreatePlaygroundSession(ctx context.Context, req openapi.CreatePlaygroundSessionRequestObject) (openapi.CreatePlaygroundSessionResponseObject, error) {
	return c.PlaygroundHandler.CreatePlaygroundSession(ctx, req)
}

// JoinPlaygroundSession delegates to the playground handler.
func (c *combinedHandler) JoinPlaygroundSession(ctx context.Context, req openapi.JoinPlaygroundSessionRequestObject) (openapi.JoinPlaygroundSessionResponseObject, error) {
	return c.PlaygroundHandler.JoinPlaygroundSession(ctx, req)
}

// GetPlaygroundSession delegates to the playground handler.
func (c *combinedHandler) GetPlaygroundSession(ctx context.Context, req openapi.GetPlaygroundSessionRequestObject) (openapi.GetPlaygroundSessionResponseObject, error) {
	return c.PlaygroundHandler.GetPlaygroundSession(ctx, req)
}

// GetPlaygroundTombstone delegates to the playground handler.
func (c *combinedHandler) GetPlaygroundTombstone(ctx context.Context, req openapi.GetPlaygroundTombstoneRequestObject) (openapi.GetPlaygroundTombstoneResponseObject, error) {
	return c.PlaygroundHandler.GetPlaygroundTombstone(ctx, req)
}

// GetPortalInfo delegates to the portalinfo handler.
func (c *combinedHandler) GetPortalInfo(ctx context.Context, req openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
	return c.PortalInfoHandler.GetPortalInfo(ctx, req)
}

// compile-time assertion that combinedHandler satisfies the full interface.
var _ openapi.StrictServerInterface = (*combinedHandler)(nil)

func main() {
	cfgPath := flag.String("config", "", "path to YAML config file (env vars override)")
	flag.Parse()

	// Load config before wiring logging — use the stdlib default logger for
	// early fatal errors (they won't be structured, but startup errors are rare).
	cfg, err := config.Load(*cfgPath)
	if err != nil {
		slog.Error("config load failed", "err", err)
		os.Exit(2)
	}

	// Wire the default slog logger now that we have log config.
	logging.Setup(cfg.Log.Format, cfg.Log.Level)

	// Build the Prometheus metrics registry. Constructed early so it can be
	// threaded into all subsystems before they start.
	metricsReg := metrics.New()

	slog.Info("portal starting",
		"bind", cfg.Bind,
		"tls_mode", cfg.TLS.Mode,
		"db_driver", cfg.DBDriver,
		"log_format", cfg.Log.Format,
		"log_level", cfg.Log.Level,
	)

	// Signal-aware context: SIGINT (Ctrl-C) or SIGTERM triggers graceful shutdown.
	ctx, stop := signal.NotifyContext(context.Background(),
		os.Interrupt, syscall.SIGTERM)
	defer stop()

	// shutdownStartCh delivers the moment ctx is cancelled to the drain block
	// below. Using a buffered channel of size 1 gives a proper happens-before
	// edge between the send (in the goroutine) and the receive (after
	// server.Run returns), satisfying the Go memory model. The channel is
	// buffered so the goroutine never blocks if server.Run exits via a listen
	// error (ctx never cancelled → channel is never sent on → drain skipped
	// via the default branch of the select below).
	shutdownStartCh := make(chan time.Time, 1)
	go func() {
		<-ctx.Done()
		shutdownStartCh <- time.Now()
	}()

	// Open the database and run migrations. Translate config.DBConfig into
	// db.PoolConfig at the call site to avoid an import cycle (internal/db
	// must not import internal/portal/config).
	dbStore, sqlDB, err := db.Open(ctx, cfg.DBDriver, cfg.DBDSN, db.PoolConfig{
		MaxOpenConns:    cfg.DB.MaxOpenConns,
		MaxIdleConns:    cfg.DB.MaxIdleConns,
		ConnMaxLifetime: cfg.DB.ConnMaxLifetime,
	})
	if err != nil {
		slog.Error("database open failed", "err", err)
		os.Exit(1)
	}
	defer dbStore.Close()
	if sqlDB != nil {
		defer sqlDB.Close()
	}

	// Playground provisioning: when enabled, seed the reserved `playground` org
	// row idempotently on every boot. If an unprotected org already holds the
	// slug "playground" (slug collision with a pre-existing user org), the portal
	// refuses to start until the operator resolves the conflict — the error
	// message includes the conflicting org's ID and the remediation steps.
	if cfg.PlaygroundEnabled {
		if err := playground.ProvisionReservedOrg(ctx, dbStore, time.Now().UTC(), slog.Default()); err != nil {
			if errors.Is(err, playground.ErrReservedSlugConflict) {
				slog.Error("playground enabled but reserved slug is taken — refusing to start",
					"err", err,
					"remediation", "rename the existing 'playground' org or set JAMSESH_PLAYGROUND_ENABLED=false")
				os.Exit(1)
			}
			// Other errors (transient DB failure): fail fast; operator can fix DB and restart.
			slog.Error("playground org provisioning failed", "err", err)
			os.Exit(1)
		}
	}

	// Determine the pod ID used to identify this instance in the distributed
	// lease table. Use the HOSTNAME env var (set by Kubernetes as the pod name)
	// with a fallback to the OS hostname. In single-instance mode this value is
	// not used (NoopManager is selected), but computing it unconditionally keeps
	// the startup sequence clean.
	podID := os.Getenv("HOSTNAME")
	if podID == "" {
		if h, err := os.Hostname(); err == nil {
			podID = h
		} else {
			podID = "unknown-pod"
		}
	}

	// Build and wire the session lease manager. In single-instance mode
	// (DeployMode="single", the default) this returns a NoopManager that never
	// touches the leases table. In clustered mode it returns a PostgresManager
	// that uses advisory locks and fencing tokens.
	heartbeatInterval := time.Duration(cfg.LeaseHeartbeatIntervalS) * time.Second
	leaseMgr := lease.New(cfg.DeployMode, sqlDB, dbStore, podID, heartbeatInterval, metricsReg)

	// In clustered mode, start the retention goroutine that purges old released
	// lease rows. The goroutine respects ctx cancellation so it exits on SIGTERM.
	if cfg.DeployMode == "clustered" {
		retentionInterval := time.Duration(cfg.LeaseRetentionIntervalHours) * time.Hour
		retentionDuration := time.Duration(cfg.LeaseRetentionDays) * 24 * time.Hour
		go func() {
			if err := lease.RunRetention(ctx, dbStore, retentionInterval, retentionDuration, func() time.Time { return time.Now().UTC() }); err != nil {
				slog.Info("lease retention goroutine stopped", "reason", err)
			}
		}()
	}

	// Log the deploy mode and the lease manager type for observability.
	slog.Info("lease manager configured",
		"deploy_mode", cfg.DeployMode,
		"manager_type", fmt.Sprintf("%T", leaseMgr),
		"pod_id", podID,
	)

	// Build the email sender from config. A missing / misconfigured provider
	// is a startup error, not a runtime one.
	emailSender, err := senders.New(cfg.Email)
	if err != nil {
		slog.Error("email sender init failed", "err", err)
		os.Exit(1)
	}

	// Build the test-clock provider. In e2etest builds this returns a
	// provider holding an AdvanceableClock that's injected into handlers
	// requiring time.Now indirection (magic-link AND tokens — advancing
	// once moves both forward). In production builds (no -tags e2etest)
	// this returns a no-op stub: magicLinkClock() / tokensClock() are nil
	// and mountTestEndpointsHook() is nil, so the /test subtree is never
	// registered.
	testClk := newTestClockProvider()

	// Build the token service. In e2etest builds, inject the advanceable
	// clock so /test/clock-advance affects token-expiry validation
	// (un-blocks tests/e2e/chaos/runtime_and_clock_test.go >
	// clock_skew_token_expiry). In production builds the provider's
	// tokensClock() returns nil and the real-clock constructor is used.
	var tokenSvc tokens.Service
	if c := testClk.tokensClock(); c != nil {
		tokenSvc = tokens.NewWithClock(dbStore, c)
	} else {
		tokenSvc = tokens.New(dbStore)
	}
	tokenHandler := tokens.NewHandler(tokenSvc)

	// Build the magic-link handler. In e2etest builds, inject the
	// advanceable clock; in production builds, the provider's
	// magicLinkClock() returns nil and the real-clock constructor is used.
	var magicLinkHandler *auth.MagicLinkHandler
	if c := testClk.magicLinkClock(); c != nil {
		magicLinkHandler = auth.NewMagicLinkHandlerWithClock(dbStore, tokenSvc, emailSender, cfg.PortalURL, c)
	} else {
		magicLinkHandler = auth.NewMagicLinkHandler(dbStore, tokenSvc, emailSender, cfg.PortalURL)
	}

	// Build the OAuth provider map. GitHub is configured when both ClientID
	// and ClientSecret are non-empty; otherwise the entry is nil and the
	// start endpoint returns 503 when invoked for that provider.
	providers := map[string]portaloauth.Provider{
		"github": nil, // registered but unconfigured by default
	}
	if cfg.OAuth.GitHub.ClientID != "" && cfg.OAuth.GitHub.ClientSecret != "" {
		providers["github"] = portaloauth.NewGitHub(portaloauth.GitHubOptions{
			ClientID:     cfg.OAuth.GitHub.ClientID,
			ClientSecret: cfg.OAuth.GitHub.ClientSecret,
			BaseURL:      cfg.OAuth.GitHub.BaseURL,
		})
	}

	// Build the OAuth handler.
	oauthHandler := auth.NewOAuthHandler(providers, dbStore, tokenSvc, cfg.PortalURL)

	// Build the accounts handler (GET /api/me, POST /api/orgs, org members + invites).
	// In e2etest builds, inject the advanceable clock so /test/clock-advance
	// affects org-invite expiry and CreatedAt/ExpiresAt stamps; in production
	// builds the provider's accountsClock() returns nil and the real-clock
	// constructor is used.
	var accountsHandler *accounts.Handler
	if c := testClk.accountsClock(); c != nil {
		accountsHandler = accounts.NewWithClock(dbStore, emailSender, cfg.PortalURL, c)
	} else {
		accountsHandler = accounts.New(dbStore, emailSender, cfg.PortalURL)
	}

	// Best-effort: ensure the top-level storage directory exists so the /readyz
	// storage check (os.Stat on cfg.Storage) passes on fresh installs. Without
	// this the probe returns 503 until the first push lazily creates a session
	// repo's parent dirs. We log-and-continue on failure so this doesn't mask
	// other startup-fast-fail paths (e.g. clustered-mode object-storage
	// unreachable, which the operator and tests want to see surface first);
	// if MkdirAll fails here, the readyz storage check will fail at request
	// time with the same underlying error in its response body.
	if err := os.MkdirAll(cfg.Storage, 0o750); err != nil {
		slog.Warn("storage dir create failed; /readyz storage check will report it",
			"path", cfg.Storage, "err", err)
	}

	// Build the storage service and git HTTP handler (needed before sessions).
	// In e2etest builds, inject the advanceable clock so archive timestamps
	// reflect /test/clock-advance offsets; in production builds the
	// provider's storageClock() returns nil and the real-clock constructor
	// is used.
	var storageSvc storage.Service
	if c := testClk.storageClock(); c != nil {
		storageSvc = storage.NewWithClock(cfg.Storage, dbStore, c)
	} else {
		storageSvc = storage.New(cfg.Storage, dbStore)
	}

	// Build the event log. In e2etest builds, inject the advanceable clock
	// so event CreatedAt timestamps reflect the advanced offset; in
	// production builds the provider's eventsClock() returns nil and the
	// real-clock constructor is used. Metrics registry threaded in regardless.
	var eventLog *events.Log
	if c := testClk.eventsClock(); c != nil {
		eventLog = events.NewWithClock(dbStore, c).WithMetrics(metricsReg)
	} else {
		eventLog = events.New(dbStore).WithMetrics(metricsReg)
	}

	// Build the object-storage sync pipeline when running in clustered mode.
	// In single-instance mode (the default) this block is skipped entirely and
	// a nil Syncer is passed to the postreceive Emitter, which treats nil as
	// "no sync" — local disk remains the system of record.
	//
	// In clustered mode, the Syncer mirrors every push to object storage before
	// the git client receives a success response (RPO=0 contract). config.validate()
	// already guarantees ObjectStorageURL is non-empty in clustered mode, so the
	// guard below is belt-and-suspenders.
	var objSyncer *objectstore.Syncer
	var objLifecycle *objectstore.LifecycleManager
	if cfg.DeployMode == "clustered" && cfg.ObjectStorageURL != "" {
		backend, backendErr := objectstore.New(cfg.ObjectStorageURL, objectstore.Config{
			Region:       cfg.ObjectStorageRegion,
			EndpointURL:  cfg.ObjectStorageEndpointURL,
			UsePathStyle: cfg.ObjectStoragePathStyle,
		})
		if backendErr != nil {
			slog.Error("object storage backend init failed", "err", backendErr,
				"url", cfg.ObjectStorageURL)
			os.Exit(1)
		}
		slog.Info("object storage backend initialised",
			"url", cfg.ObjectStorageURL,
			"region", cfg.ObjectStorageRegion,
			"path_style", cfg.ObjectStoragePathStyle,
		)

		// Fail fast: probe the bucket before the HTTP listener starts.
		// A 5-second timeout is conservative for a single HEAD request; on
		// success the probe returns in < 100 ms. On an unreachable endpoint the
		// context times out and the process exits non-zero before accepting traffic.
		probeCtx, probeCancel := context.WithTimeout(ctx, 5*time.Second)
		probeErr := backend.Probe(probeCtx)
		probeCancel()
		if probeErr != nil {
			slog.Error("object storage connectivity check failed",
				"err", probeErr,
				"url", cfg.ObjectStorageURL,
			)
			os.Exit(1)
		}
		slog.Info("object storage probe succeeded", "url", cfg.ObjectStorageURL)

		objSyncer = &objectstore.Syncer{
			Backend:                backend,
			Manifests:              &objectstore.ManifestStore{Backend: backend},
			Storage:                storageSvc,
			Metrics:                metricsReg,
			QueueSize:              cfg.ObjectStorageSyncQueueSize,
			PerSessionBackpressure: true,
		}

		// Build the LifecycleManager: per-pod session lease + hydration
		// coordinator. On the first request for a session this pod doesn't hold,
		// AcquireForRequest acquires the distributed lease and downloads the bare
		// repo from object storage before the push is processed. On release (idle
		// timeout, LRU cap, lease loss, or SIGTERM), it drains in-flight uploads
		// and removes the local cache.
		hydrator := &objectstore.Hydrator{
			Backend:   backend,
			Manifests: &objectstore.ManifestStore{Backend: backend},
			Storage:   storageSvc,
			Metrics:   metricsReg,
			Workers:   cfg.HydrationWorkers,
		}
		objLifecycle = &objectstore.LifecycleManager{
			Lease:    leaseMgr,
			Hydrator: hydrator,
			Syncer:   objSyncer,
			Storage:  storageSvc,
			OrgIDLookup: func(ctx context.Context, sessionID string) (string, error) {
				sess, err := dbStore.GetSessionByID(ctx, sessionID)
				if err != nil {
					return "", fmt.Errorf("orgID lookup for session %s: %w", sessionID, err)
				}
				return sess.OrgID, nil
			},
			IdleTimeout:     time.Duration(cfg.HydrationIdleTimeoutS) * time.Second,
			CacheMaxBytes:   cfg.HydrationCacheMaxBytes,
			IdleCheckPeriod: time.Duration(cfg.HydrationIdleCheckPeriodS) * time.Second,
			Metrics:         metricsReg,
		}

		// Start the lifecycle goroutine: idle-eviction + LRU loop. Blocks until
		// ctx is cancelled (SIGTERM), then drains all active sessions.
		go func() {
			if err := objLifecycle.Start(ctx); err != nil && err != context.Canceled {
				slog.Warn("lifecycle manager exited", "err", err)
			}
		}()

		slog.Info("lifecycle manager started",
			"idle_timeout_s", cfg.HydrationIdleTimeoutS,
			"cache_max_bytes", cfg.HydrationCacheMaxBytes,
			"idle_check_period_s", cfg.HydrationIdleCheckPeriodS,
			"workers", cfg.HydrationWorkers,
		)
	}

	// Build the playground config once; shared by playground handler, destruction
	// worker, rate limiter, git handler, comments service, and sessions handler.
	pgCfg := playground.Config{
		Enabled:         cfg.PlaygroundEnabled,
		IdleTimeout:     time.Duration(cfg.PlaygroundIdleTimeoutS) * time.Second,
		HardCap:         time.Duration(cfg.PlaygroundHardCapS) * time.Second,
		MaxParticipants: cfg.PlaygroundMaxParticipants,
		CreatePerIPHour: cfg.PlaygroundCreatePerIPHour,
		MaxContentBytes: cfg.PlaygroundMaxContentBytes,
	}

	// Build the sessions handler (lifecycle + invites + member-remove endpoints).
	// In e2etest builds, inject the advanceable clock so /test/clock-advance
	// affects session created_at / ended_at stamps, invite created/expires/
	// accepted/joined stamps, and the listing cursor "before" window.
	var sessionsHandler *sessions.Handler
	if c := testClk.sessionsClock(); c != nil {
		sessionsHandler = sessions.NewWithClock(dbStore, storageSvc, eventLog, emailSender, cfg.PortalURL, c)
	} else {
		sessionsHandler = sessions.New(dbStore, storageSvc, eventLog, emailSender, cfg.PortalURL)
	}
	// Wire the playground idle timeout so FinalizeSession can reset the idle
	// timer for playground sessions (substantive-event activity tracking).
	sessionsHandler = sessionsHandler.WithPlaygroundIdleTimeout(pgCfg.IdleTimeout)

	// Build the playground handler. The handler is always constructed (even
	// when PlaygroundEnabled=false) because the routes are registered
	// unconditionally — the handler returns 503 for all calls when disabled.
	// In e2etest builds, playgroundClock() returns the shared AdvanceableClock
	// so POST /test/clock-advance moves session expiry checks forward; in
	// production it returns nil and we fall back to playground.RealClock().
	playgroundClock := playground.Clock(playground.RealClock())
	if c := testClk.playgroundClock(); c != nil {
		playgroundClock = c
	}
	playgroundHandler := &playground.Handler{
		Store:   dbStore,
		Tokens:  tokenSvc,
		Storage: storageSvc,
		Cfg:     pgCfg,
		Clock:   playgroundClock,
		Logger:  slog.Default(),
	}

	// Build the comments service and handler. In e2etest builds, the
	// Clock field is set to the advanceable clock via the struct-literal;
	// in production builds commentsClock() returns nil and the now()
	// helper falls back to the real wall clock.
	commentsSvc := &comments.Service{
		Store:                 dbStore,
		Log:                   eventLog,
		Clock:                 testClk.commentsClock(),
		PlaygroundIdleTimeout: pgCfg.IdleTimeout,
	}
	commentsHandler := comments.NewHandler(commentsSvc)

	// Build the finalize handler (lock-CRUD endpoints; plan/fetch-token/
	// mark-shipped land in subsequent stories of epic-finalize-flow).
	// In e2etest builds, inject the advanceable clock so /test/clock-advance
	// affects the 30-minute idle-lock check and write timestamps.
	var finalizeHandler *finalize.Handler
	if c := testClk.finalizeClock(); c != nil {
		finalizeHandler = finalize.NewWithClock(dbStore, storageSvc, eventLog, tokenSvc, cfg.PortalURL, c)
	} else {
		finalizeHandler = finalize.New(dbStore, storageSvc, eventLog, tokenSvc, cfg.PortalURL)
	}

	// Build the session-resume handler (mint + exchange endpoints). In e2etest
	// builds, inject the advanceable clock so /test/clock-advance affects the
	// 60-second resume-token TTL check. Mirrors the finalize handler wiring above.
	var sessionResumeHandler *sessionresume.Handler
	if c := testClk.sessionresumeClock(); c != nil {
		sessionResumeHandler = sessionresume.NewWithClock(dbStore, tokenSvc, cfg.PortalURL, c)
	} else {
		sessionResumeHandler = sessionresume.New(dbStore, tokenSvc, cfg.PortalURL)
	}

	// Build the MCP endpoint. Auth is handled by the SDK's
	// auth.RequireBearerToken middleware wired inside Handler(). In e2etest
	// builds, the Clock field is set to the advanceable clock via the
	// struct-literal; in production builds mcpClock() returns nil and the
	// endpoint's now() helper falls back to the real wall clock.
	mcpEndpoint := &mcpendpoint.Endpoint{
		Store:    dbStore,
		Tokens:   tokenSvc,
		Storage:  storageSvc,
		Log:      eventLog,
		Comments: commentsSvc,
		Clock:    testClk.mcpClock(),
	}

	// Start the playground destruction worker when playground is enabled.
	// The worker sweeps for expired sessions every PlaygroundDestructionSweepIntervalS
	// seconds and runs the idempotent 8-step cascade for each expired session.
	// It participates in graceful shutdown via ctx cancellation: when ctx is
	// cancelled (SIGTERM), the ticker loop exits on the next fire (within one
	// Interval window). This mirrors the lifecycle goroutine above.
	if cfg.PlaygroundEnabled {
		// Same playgroundClock as the handler — advancing once moves both
		// forward so the worker's per-sweep "what's expired" query agrees
		// with the handler's hard-cap / idle-timeout reads.
		destructionWorker := &playground.Worker{
			Store:    dbStore,
			Storage:  storageSvc,
			Cfg:      pgCfg,
			Clock:    playgroundClock,
			Interval: time.Duration(cfg.PlaygroundDestructionSweepIntervalS) * time.Second,
			Logger:   slog.Default(),
			Leases:   leaseMgr,
		}
		go func() {
			if err := destructionWorker.Run(ctx); err != nil && !errors.Is(err, context.Canceled) {
				slog.Error("playground destruction worker exited with error", "err", err)
			}
		}()
		slog.Info("playground destruction worker started",
			"sweep_interval_s", cfg.PlaygroundDestructionSweepIntervalS)
	}

	// Build and start the auto-merger worker. It subscribes to commit.arrived
	// events and runs merge + apply in per-session goroutines.
	// In e2etest builds, inject the advanceable clock so the merger
	// signature timestamp and conflict event/resolve timestamps reflect
	// /test/clock-advance offsets.
	portalHost := cfg.PortalURL
	var mergerApplier *automerger.Applier
	if c := testClk.automergerClock(); c != nil {
		mergerApplier = automerger.NewApplierWithClock(dbStore, eventLog, c)
	} else {
		mergerApplier = automerger.NewApplier(dbStore, eventLog)
	}
	mergerApplier.Metrics = metricsReg
	mergerWorker := &automerger.Worker{
		Store:      dbStore,
		Storage:    storageSvc,
		Log:        eventLog,
		Applier:    mergerApplier,
		PortalHost: portalHost,
		Metrics:    metricsReg,
	}
	if err := mergerWorker.Start(ctx); err != nil {
		slog.Error("auto-merger worker start failed", "err", err)
		os.Exit(1)
	}

	// Build and start the WebSocket gateway. It subscribes to all events and
	// fans them out to connected clients at /ws/sessions/{sessionID}.
	//
	// AllowOrigins is parsed from JAMSESH_WS_ALLOW_ORIGINS (comma-separated
	// origins). Defaults to empty (deny all cross-origin upgrades) when unset.
	// See docs/SELF_HOST.md for the configuration table.
	//
	// The ticket store issues short-lived (60-second) single-use upgrade tickets
	// via POST /api/auth/ws-ticket. The SPA fetches a ticket immediately before
	// opening the WebSocket so the long-lived bearer token never appears in
	// Sec-WebSocket-Protocol.
	wsAllowOrigins := parseAllowOrigins(os.Getenv("JAMSESH_WS_ALLOW_ORIGINS"))
	wsTicketStore := wsgateway.NewTicketStore()
	wsTicketStore.Start()
	wsGateway := &wsgateway.Gateway{
		Store:        dbStore,
		Tickets:      wsTicketStore,
		Log:          eventLog,
		AllowOrigins: wsAllowOrigins,
	}
	if err := wsGateway.Start(ctx); err != nil {
		slog.Error("ws gateway start failed", "err", err)
		os.Exit(1)
	}

	// Compose the combined handler that satisfies the full StrictServerInterface.
	// The strict-handler options funnel every handler error and request-decode
	// error through the PROTOCOL.md envelope helpers in internal/portal/httperr:
	//   - ResponseErrorHandlerFunc: translates *httperr.Error (pass-through),
	//     deperr sentinels (typed dep envelopes), or anything else (fallthrough
	//     to "internal" 500) via httperr.WriteFromError.
	//   - RequestErrorHandlerFunc: emits a "request.malformed" 400 envelope
	//     instead of the default plain-text response.
	portalInfoHandler, err := portalinfo.NewHandler(cfg.PlaygroundEnabled, cfg.Landing.Variant)
	if err != nil {
		slog.Error("portalinfo handler init failed", "err", err, "landing_variant", cfg.Landing.Variant)
		os.Exit(1)
	}

	strictAPI := openapi.NewStrictHandlerWithOptions(&combinedHandler{
		Handler:           tokenHandler,
		MagicLinkHandler:  magicLinkHandler,
		OAuthHandler:      oauthHandler,
		AccountsHandler:   accountsHandler,
		SessionsHandler:   sessionsHandler,
		CommentsHandler:   commentsHandler,
		FinalizeHandler:      finalizeHandler,
		SessionResumeHandler: sessionResumeHandler,
		WsTicketHandler:      &wsgateway.WsTicketHandler{Tickets: wsTicketStore},
		PlaygroundHandler: playgroundHandler,
		PortalInfoHandler: portalInfoHandler,
	}, nil, openapi.StrictHTTPServerOptions{
		RequestErrorHandlerFunc:  httperr.WriteBadRequest,
		ResponseErrorHandlerFunc: httperr.WriteFromError,
	})

	// Build a ServerInterfaceWrapper so we have http.HandlerFunc-compatible
	// method values for all routes, including those with URL path parameters.
	// The wrapper's ErrorHandlerFunc fires when path/query parameter binding
	// fails — also route through the standard 400 envelope.
	apiWrapper := &openapi.ServerInterfaceWrapper{
		Handler:          strictAPI,
		ErrorHandlerFunc: httperr.WriteBadRequest,
	}

	// Build the per-instance receive-pack concurrency semaphore.
	// Buffer size = ReceivePackMaxConcurrent (default 4). Handlers acquire a
	// slot on entry and release on exit; when full, new pushes receive 503
	// Retry-After. This bounds concurrent-push RSS independent of the per-pack
	// cap: at most N packs are in-flight at once, each spilling to a tempfile.
	receivePackSem := make(chan struct{}, cfg.Git.ReceivePackMaxConcurrent)

	gitHandler := &githttp.Handler{
		Store:   dbStore,
		Tokens:  tokenSvc,
		Storage: storageSvc,
		Validator: &prereceive.Validator{
			MaxPackBytes:              cfg.Git.MaxPackBytes,
			PlaygroundMaxContentBytes: cfg.PlaygroundMaxContentBytes,
		},
		Emitter: &postreceive.Emitter{
			Log:       eventLog,
			Syncer:    objSyncer,    // nil in single-instance mode; Emitter handles nil as no-op
			Lifecycle: objLifecycle, // nil in single-instance mode; provides hydration + long-held lease
			Storage:   storageSvc,  // used only when Syncer is non-nil
		},
		Metrics:               metricsReg,
		ReceivePackSem:        receivePackSem,
		PlaygroundIdleTimeout: pgCfg.IdleTimeout,
		Clock:                 githttp.RealClock(),
	}
	// Set Lifecycle only when objLifecycle is non-nil. Assigning a
	// typed nil *LifecycleManager to the lifecycleAcquirer interface field
	// would make the interface non-nil (Go nil-interface trap), causing the
	// handler's nil check to pass and the subsequent method call to panic
	// with "invalid memory address" on every single-mode git request.
	if objLifecycle != nil {
		gitHandler.Lifecycle = objLifecycle
	}

	// Wire the embedded SPA handler. assets.Handler() returns a handler that
	// serves the compiled Svelte bundle from frontend/dist/ with a History-API
	// fallback to index.html. If the build hasn't run yet (dist/ only has
	// .gitkeep), deep links return a blank 200 — acceptable for dev; CI always
	// runs npm run build before go build.
	uiHandler, err := assets.Handler()
	if err != nil {
		slog.Error("assets handler init failed", "err", err)
		os.Exit(1)
	}

	// Build the chi router. MountAPI registers route groups by auth requirement:
	//   - Public (no Bearer): /auth/refresh, /auth/magic-link/request,
	//                          /auth/magic-link/exchange
	//   - Bearer: /auth/revoke (and future authenticated endpoints)
	handler := router.New(router.Deps{
		Security: router.Security{
			TLSMode:           cfg.TLS.Mode,
			TrustProxyHeaders: cfg.TLS.Mode == "behind_proxy",
		},
		Metrics: router.Metrics{
			Handler:  metricsReg.Handler(),
			Token:    cfg.MetricsToken,
			Registry: metricsReg,
		},
		Probes: router.Probes{
			Ready: []probes.Check{
				{
					Name: "db",
					Fn:   dbStore.Ping,
				},
				{
					Name: "storage",
					Fn: func(ctx context.Context) error {
						_, err := os.Stat(cfg.Storage)
						return err
					},
				},
			},
		},
		APIBodyLimitBytes: cfg.APIBodyLimitBytes,
		Mounts: router.Mounts{
			UI:   uiHandler,
			Git:  gitHandler.Mount,
			MCP:  mcpEndpoint.Handler(),
			WS:   wsGateway.Handler(),
			Test: testClk.mountTestEndpointsHook(),
			API: func(r chi.Router) {
			// Per-IP rate limiters for each unauthenticated auth endpoint.
			// Limits: magic-link/request 3/min 10/hr; oauth/start 5/min 20/hr;
			// exchange/callback 10/min; refresh 20/min.
			// Controlled by JAMSESH_AUTH_RATE_LIMIT_ENABLED (default: true).
			rlEnabled := cfg.AuthRateLimitEnabled
			mlRequestRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 3, PerHour: 10}).Middleware(rlEnabled)
			oauthStartRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 5, PerHour: 20}).Middleware(rlEnabled)
			mlExchangeRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 10}).Middleware(rlEnabled)
			oauthCallbackRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 10}).Middleware(rlEnabled)
			refreshRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 20}).Middleware(rlEnabled)
			// Session-resume mint: 10/min per bearer account — generous enough
			// for normal CLI usage, tight enough to limit token-flood attacks.
			sessionResumeRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 10}).Middleware(rlEnabled)

			// Public auth endpoints — no Bearer middleware.
			r.Group(func(r chi.Router) {
				r.With(refreshRL).Post("/auth/refresh", apiWrapper.RefreshToken)
				r.With(mlRequestRL).Post("/auth/magic-link/request", apiWrapper.RequestMagicLink)
				r.With(mlExchangeRL).Post("/auth/magic-link/exchange", apiWrapper.ExchangeMagicLink)
				r.With(oauthStartRL).Post("/auth/oauth/start", apiWrapper.StartOAuth)
				r.With(oauthCallbackRL).Post("/auth/oauth/callback", apiWrapper.OauthCallback)
			})

			// Authenticated endpoints — Bearer middleware required.
			r.Group(func(r chi.Router) {
				r.Use(tokens.BearerMiddleware(tokenSvc))
				r.Post("/auth/revoke", apiWrapper.RevokeToken)
				// feature-auth-signout-backend-revoke-backend — zero-body
				// sign-out endpoint; revokes all tokens for the caller.
				r.Post("/auth/logout", apiWrapper.Logout)
				r.Post("/auth/ws-ticket", apiWrapper.IssueWsTicket)
				r.Get("/me", apiWrapper.GetMe)
				r.Post("/orgs", apiWrapper.CreateOrg)

				// Org members: requires creator or member role.
				r.Group(func(r chi.Router) {
					r.Use(auth.RequireOrgRole(dbStore, "creator", "member"))
					r.Get("/orgs/{orgID}/members", apiWrapper.ListOrgMembers)
				})

				// Org invites create: requires creator role.
				r.Group(func(r chi.Router) {
					r.Use(auth.RequireOrgRole(dbStore, "creator"))
					r.Post("/orgs/{orgID}/invites", apiWrapper.CreateOrgInvite)
				})

				// Accept invite: Bearer only — the user is joining the org,
				// so no org-role gate applies yet.
				r.Post("/orgs/{orgID}/invites/{inviteID}/accept", apiWrapper.AcceptOrgInvite)

				// Get org: auth + org-membership check is performed inside the handler.
				r.Get("/orgs/{orgID}", apiWrapper.GetOrg)

				// Patch org: auth + creator-role check is performed inside the handler.
				r.Patch("/orgs/{orgID}", apiWrapper.PatchOrg)

				// Sessions: any org member can create/list; other ops are checked in the handler.
				r.Get("/orgs/{orgID}/sessions", apiWrapper.ListSessions)
				r.Post("/orgs/{orgID}/sessions", apiWrapper.CreateSession)
				r.Get("/orgs/{orgID}/sessions/{sessionID}", apiWrapper.GetSession)
				r.Patch("/orgs/{orgID}/sessions/{sessionID}", apiWrapper.PatchSession)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/finalize", apiWrapper.FinalizeSession)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/abandon", apiWrapper.AbandonSession)
				r.Get("/orgs/{orgID}/sessions/{sessionID}/refs", apiWrapper.ListSessionRefs)
				r.Get("/orgs/{orgID}/sessions/{sessionID}/digest", apiWrapper.GetSessionDigest)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/invites", apiWrapper.InviteToSession)
				r.Get("/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}", apiWrapper.GetSessionInvite)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/invites/{inviteID}/accept", apiWrapper.AcceptSessionInvite)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/members/{accountID}/remove", apiWrapper.RemoveSessionMember)

				// Comments: any session member can list/create; resolve also requires session membership.
				r.Get("/orgs/{orgID}/sessions/{sessionID}/comments", apiWrapper.ListComments)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/comments", apiWrapper.CreateComment)
				r.Post("/orgs/{orgID}/sessions/{sessionID}/comments/{commentId}/resolve", apiWrapper.ResolveComment)

				// Files: any session member can view file content.
				r.Get("/orgs/{orgID}/sessions/{sessionID}/files", apiWrapper.GetSessionFile)

				// Ref modes: any session member can upsert ref mode.
				r.Post("/orgs/{orgID}/sessions/{sessionID}/ref-modes", apiWrapper.UpsertRefMode)

				// Finalize locks: any session member can acquire/patch/release;
				// caller-vs-holder enforcement happens in the handler.
				r.Post("/orgs/{orgID}/sessions/{sessionID}/finalize/lock", apiWrapper.AcquireFinalizeLock)
				r.Patch("/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}", apiWrapper.PatchFinalizeLock)
				r.Delete("/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}", apiWrapper.ReleaseFinalizeLock)

				// Finalize plan: any session member; the handler validates the
				// lock_id binding and idle/superseded state.
				r.Get("/orgs/{orgID}/sessions/{sessionID}/finalize-plan", apiWrapper.GetFinalizePlan)

				// Finalize fetch-token: mints an ephemeral fetch-only token +
				// pre-composed remote URL for the plugin's HTTPS-fallback path.
				// Session-membership enforced inside the handler.
				r.Post("/orgs/{orgID}/sessions/{sessionID}/finalize/fetch-token", apiWrapper.IssueFetchToken)

				// Mark-shipped: caller asserts the cherry-pick script ran;
				// handler validates lock_id binding and transitions the
				// session to shipped.
				r.Post("/orgs/{orgID}/sessions/{sessionID}/mark-shipped", apiWrapper.MarkSessionShipped)

				// Session resume: CLI mints a single-use 60-second resume token
				// so the browser portal can re-authenticate the CLI session.
				// Session-membership enforced inside the handler.
				r.With(sessionResumeRL).Post("/session-resumes", apiWrapper.CreateSessionResume)


				// Playground — GET session requires a valid anonymous bearer
				// (issued at create/join time). The handler validates membership.
				r.Get("/playground/sessions/{id}", apiWrapper.GetPlaygroundSession)
			})

			// Portal info — fully public, no auth or rate-limiting needed.
			// Returns deploy-time config (playground_enabled, landing_variant) for
			// anonymous SPA bootstrap before the auth flow completes.
			// Cache-Control: no-store so deploy-time toggles propagate immediately
			// (gate-security-portalinfo-no-cachecontrol-no-store).
			r.With(portalinfo.NoCacheMiddleware).Get("/portal/info", apiWrapper.GetPortalInfo)

			// Session-resume exchange: unauthenticated — the resume token IS the
			// credential. Rate-limited per source IP to prevent replay-flood attacks
			// against the single-use consume path.
			sessionResumeExchangeRL := ratelimit.NewStore(ratelimit.Config{PerMinute: 10}).Middleware(rlEnabled)
			r.With(sessionResumeExchangeRL).Post("/session-resumes/exchange", apiWrapper.ExchangeSessionResume)

			// Playground — unauthenticated: create and join issue fresh bearers,
			// tombstone is public (no credential needed to read destruction summary).
			// CreatePlaygroundSession is rate-limited per source IP to prevent
			// drive-by abuse. Join and tombstone are NOT rate-limited: joining
			// requires a valid session ID (already gated by participant cap), and
			// tombstone reads are idempotent cheap GETs.
			pgCreateRL := playground.CreateRateLimitMiddleware(
				playground.NewCreateRateLimiter(pgCfg),
				cfg.PlaygroundEnabled,
			)
			r.Group(func(r chi.Router) {
				r.With(pgCreateRL).Post("/playground/sessions", apiWrapper.CreatePlaygroundSession)
				r.Post("/playground/sessions/{id}/join", apiWrapper.JoinPlaygroundSession)
				r.Get("/playground/sessions/{id}/tombstone", apiWrapper.GetPlaygroundTombstone)
			})
		},
		},
	})

	if err := server.Run(ctx, cfg, handler); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}

	// Drain subsystems within the remaining grace budget.
	//
	// server.Run blocks until ctx is cancelled (graceful path) or a listen
	// error occurs (error path, already handled above). On the graceful path,
	// shutdownStartCh will have exactly one value: the timestamp recorded when
	// ctx fired. server.Run consumed some of the grace budget draining
	// in-flight HTTP requests; we log that elapsed time and compute how much
	// remains for the auto-merger and WS gateway.
	//
	// On the listen-error path ctx is never cancelled, so shutdownStartCh
	// holds no value — the default branch skips the drain entirely.
	select {
	case shutdownStart := <-shutdownStartCh:
		httpElapsed := time.Since(shutdownStart)
		slog.Info("shutdown", "shutdown_step", "http", "elapsed_ms", httpElapsed.Milliseconds())

		// Compute remaining budget; enforce a 1s floor so callers always get
		// at least a minimal window even if HTTP draining consumed most of it.
		grace := time.Duration(cfg.ShutdownGraceSeconds) * time.Second
		remaining := grace - httpElapsed
		if remaining < 1*time.Second {
			remaining = 1 * time.Second
		}

		// Drain the auto-merger and WS gateway in parallel so neither waits on
		// the other. Both share the same remaining budget context.
		stopCtx, cancelStop := context.WithTimeout(context.Background(), remaining)
		defer cancelStop()

		var wg sync.WaitGroup
		wg.Add(2)

		go func() {
			defer wg.Done()
			start := time.Now()
			if err := mergerWorker.Stop(stopCtx); err != nil {
				slog.Warn("auto-merger stop timed out", "err", err)
			}
			slog.Info("shutdown", "shutdown_step", "automerger",
				"elapsed_ms", time.Since(start).Milliseconds())
		}()

		go func() {
			defer wg.Done()
			start := time.Now()
			wsGateway.Stop()
			slog.Info("shutdown", "shutdown_step", "wsgateway",
				"elapsed_ms", time.Since(start).Milliseconds())
		}()

		wg.Wait()
	default:
		// Listen-error path: ctx was never cancelled, no drain needed.
	}

	slog.Info("portal stopped cleanly")
}
