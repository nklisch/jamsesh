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
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db"
	"jamsesh/internal/portal/assets"
	"jamsesh/internal/portal/config"
	"jamsesh/internal/portal/logging"
	"jamsesh/internal/portal/router"
	"jamsesh/internal/portal/server"
	"jamsesh/internal/portal/tokens"
)

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

	// Open the database and run migrations.
	dbStore, err := db.Open(ctx, cfg.DBDriver, cfg.DBDSN)
	if err != nil {
		slog.Error("database open failed", "err", err)
		os.Exit(1)
	}
	defer dbStore.Close()

	// Build the token service and its HTTP handler.
	tokenSvc := tokens.New(dbStore)
	tokenHandler := tokens.NewHandler(tokenSvc)
	strictAPI := openapi.NewStrictHandler(tokenHandler, nil)

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

	// Build the chi router. MountAPI registers two route groups:
	//   - Public:  POST /auth/refresh   (no Bearer middleware; refresh token IS the cred)
	//   - Bearer:  POST /auth/revoke    (Bearer middleware validates the access token)
	// Future authenticated endpoints join the Bearer group.
	handler := router.New(router.Deps{
		TrustProxyHeaders: cfg.TLS.Mode == "behind_proxy",
		MountUI:           uiHandler,
		MountAPI: func(r chi.Router) {
			// Public auth endpoints — no Bearer middleware.
			r.Group(func(r chi.Router) {
				r.Post("/auth/refresh", strictAPI.RefreshToken)
			})

			// Authenticated endpoints — Bearer middleware required.
			r.Group(func(r chi.Router) {
				r.Use(tokens.BearerMiddleware(tokenSvc))
				r.Post("/auth/revoke", strictAPI.RevokeToken)
			})
		},
	})

	if err := server.Run(ctx, cfg, handler); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}

	slog.Info("portal stopped cleanly")
}
