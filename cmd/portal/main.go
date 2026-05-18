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
	"jamsesh/internal/portal/postreceive"
	"jamsesh/internal/portal/prereceive"
	"jamsesh/internal/portal/probes"
	"jamsesh/internal/portal/router"
	"jamsesh/internal/portal/senders"
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
	AccountsHandler  *accounts.Handler
	SessionsHandler  *sessions.Handler
	CommentsHandler  *comments.Handler
	FinalizeHandler  *finalize.Handler
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

// MarkSessionShipped delegates to the finalize handler.
func (c *combinedHandler) MarkSessionShipped(ctx context.Context, req openapi.MarkSessionShippedRequestObject) (openapi.MarkSessionShippedResponseObject, error) {
	return c.FinalizeHandler.MarkSessionShipped(ctx, req)
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

	// shutdownClock records the moment ctx is cancelled so we can derive a
	// shared drain budget across all subsystems. It is written exactly once by
	// the goroutine below and read only after server.Run returns — no mutex
	// needed because server.Run blocks until ctx is cancelled (or a listen
	// error), creating the necessary happens-before relationship.
	var shutdownStart time.Time
	var shutdownOnce sync.Once
	go func() {
		<-ctx.Done()
		shutdownOnce.Do(func() { shutdownStart = time.Now() })
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
			if err := lease.RunRetention(ctx, dbStore, retentionInterval, retentionDuration); err != nil {
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

	// Build the comments service and handler. In e2etest builds, the
	// Clock field is set to the advanceable clock via the struct-literal;
	// in production builds commentsClock() returns nil and the now()
	// helper falls back to the real wall clock.
	commentsSvc := &comments.Service{Store: dbStore, Log: eventLog, Clock: testClk.commentsClock()}
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
	wsAllowOrigins := parseAllowOrigins(os.Getenv("JAMSESH_WS_ALLOW_ORIGINS"))
	wsGateway := &wsgateway.Gateway{
		Store:        dbStore,
		Tokens:       tokenSvc,
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
	strictAPI := openapi.NewStrictHandlerWithOptions(&combinedHandler{
		Handler:          tokenHandler,
		MagicLinkHandler: magicLinkHandler,
		OAuthHandler:     oauthHandler,
		AccountsHandler:  accountsHandler,
		SessionsHandler:  sessionsHandler,
		CommentsHandler:  commentsHandler,
		FinalizeHandler:  finalizeHandler,
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

	gitHandler := &githttp.Handler{
		Store:   dbStore,
		Tokens:  tokenSvc,
		Storage: storageSvc,
		Validator: &prereceive.Validator{
			MaxPackBytes: cfg.Git.MaxPackBytes,
		},
		Emitter: &postreceive.Emitter{
				Log:       eventLog,
				Syncer:    objSyncer,    // nil in single-instance mode; Emitter handles nil as no-op
				Lifecycle: objLifecycle, // nil in single-instance mode; provides hydration + long-held lease
				Storage:   storageSvc,  // used only when Syncer is non-nil
			},
		Metrics: metricsReg,
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
		TrustProxyHeaders: cfg.TLS.Mode == "behind_proxy",
		MountUI:           uiHandler,
		MountGit:          gitHandler.Mount,
		MountMCP:          mcpEndpoint.Handler(),
		MountWS:           wsGateway.Handler(),
		MountTest:         testClk.mountTestEndpointsHook(),
		MetricsHandler:    metricsReg.Handler(),
		MetricsRegistry:   metricsReg,
		ReadyzChecks: []probes.Check{
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
		MountAPI: func(r chi.Router) {
			// Public auth endpoints — no Bearer middleware.
			r.Group(func(r chi.Router) {
				r.Post("/auth/refresh", apiWrapper.RefreshToken)
				r.Post("/auth/magic-link/request", apiWrapper.RequestMagicLink)
				r.Post("/auth/magic-link/exchange", apiWrapper.ExchangeMagicLink)
				r.Post("/auth/oauth/start", apiWrapper.StartOAuth)
				r.Post("/auth/oauth/callback", apiWrapper.OauthCallback)
			})

			// Authenticated endpoints — Bearer middleware required.
			r.Group(func(r chi.Router) {
				r.Use(tokens.BearerMiddleware(tokenSvc))
				r.Post("/auth/revoke", apiWrapper.RevokeToken)
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
			})
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
	// shutdownStart was set when ctx fired, and server.Run consumed some of the
	// budget draining in-flight HTTP requests. We log the HTTP step elapsed time
	// and compute how much of the shared budget remains for the auto-merger and
	// WS gateway.
	//
	// On the listen-error path server.Run exits immediately without ctx being
	// cancelled, so shutdownStart is zero — skip the drain in that case.
	if !shutdownStart.IsZero() {
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
	}

	slog.Info("portal stopped cleanly")
}
