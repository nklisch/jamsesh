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

	"jamsesh/internal/portal/config"
	"jamsesh/internal/portal/logging"
	"jamsesh/internal/portal/router"
	"jamsesh/internal/portal/server"
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

	// Build the chi router. Mount hooks are nil until sibling features land:
	//   MountAPI  — tokens + auth-flows + accounts (epic-portal-auth)
	//   MountGit  — smart-HTTP git (epic-portal-git)
	//   MountMCP  — MCP SDK handler (epic-portal-api)
	//   MountWS   — WebSocket sessions (epic-portal-api)
	handler := router.New(router.Deps{
		TrustProxyHeaders: cfg.TLS.Mode == "behind_proxy",
	})

	if err := server.Run(ctx, cfg, handler); err != nil {
		slog.Error("server exited with error", "err", err)
		os.Exit(1)
	}

	slog.Info("portal stopped cleanly")
}
