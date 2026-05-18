// Command jamsesh-router is the jamsesh consistent-hash reverse proxy.
//
// It is an optional component deployed only in clustered mode. Single-instance
// jamsesh deployments skip it entirely and talk to portal pods directly.
//
// # What it does
//
// Every incoming HTTP request is examined for a session ID (extracted from
// the URL path for REST / Git / WebSocket, or from the Jam-Session-Id header
// for MCP connections). The session ID is used to select a portal pod via a
// consistent-hash ring, with a short-lived soft-coordinator hint cache for
// stickiness. Requests without a session ID (e.g. /healthz, /auth/*) are
// forwarded to any pod via round-robin.
//
// # Configuration
//
// Config is loaded from an optional YAML file (path given as the first
// positional argument) with environment-variable overrides. Run with
// --help for a summary of env vars; see [jamsesh/internal/router/config]
// for the full reference.
//
// # Signals
//
// SIGTERM triggers a graceful drain: the server stops accepting new
// connections and waits up to shutdown_grace_s seconds for in-flight
// requests to complete.
package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"jamsesh/internal/portal/metrics"
	"jamsesh/internal/router/cache"
	"jamsesh/internal/router/config"
	"jamsesh/internal/router/extract"
	"jamsesh/internal/router/proxy"
	"jamsesh/internal/router/readyz"
	"jamsesh/internal/router/ring"
)

func main() {
	os.Exit(run(os.Args[1:]))
}

// run is the testable entrypoint. It returns an exit code.
func run(args []string) int {
	// Parse optional config file path from first argument.
	var cfgPath string
	for _, arg := range args {
		if arg == "--help" || arg == "-h" {
			printUsage()
			return 0
		}
		// First non-flag argument is the config file path.
		if len(arg) > 0 && arg[0] != '-' {
			cfgPath = arg
			break
		}
	}

	// Load and validate configuration.
	cfg, err := config.Load(cfgPath)
	if err != nil {
		fmt.Fprintf(os.Stderr, "jamsesh-router: config: %v\n", err)
		return 2
	}

	// Initialise structured logger (JSON to stdout, matches portal convention).
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{
		Level: slog.LevelInfo,
	}))
	slog.SetDefault(logger)

	slog.Info("jamsesh-router starting",
		"bind", cfg.Bind,
		"discovery_mode", cfg.DiscoveryMode,
		"vnodes", cfg.Vnodes,
		"hint_cache_ttl", cfg.HintCacheTTL.String(),
		"shutdown_grace_s", cfg.ShutdownGraceSeconds,
	)

	// Build the Prometheus metrics registry.
	metricsReg := metrics.New()

	// Build the consistent-hash ring.
	r := ring.New(cfg.Vnodes)

	// publishWithMetrics wraps a publish callback to record ring-size gauge
	// and rebalance counter updates. This keeps the discoverer itself free of
	// metrics dependencies (Option B from the design).
	publishWithMetrics := func(base func([]ring.Pod)) func([]ring.Pod) {
		return func(pods []ring.Pod) {
			base(pods)
			metricsReg.RouterRingRebalancesTotal.Inc()
			metricsReg.RouterRingSize.Set(float64(len(pods)))
		}
	}
	_ = publishWithMetrics // used below when discovery is wired; silences unused warning

	// Build the readiness probe (with metrics wired for failure tracking).
	probe := &readyz.Probe{
		Metrics: metricsReg,
	}
	_ = probe // used by discovery; probe is constructed here for metrics wiring

	// Seed the ring from static config (the discovery story / Unit 3 will
	// overlay this with live k8s watch results by calling SetPods on the
	// same ring pointer).
	if cfg.DiscoveryMode == "static" {
		pods := make([]ring.Pod, 0, len(cfg.StaticPods))
		for _, addr := range cfg.StaticPods {
			pods = append(pods, ring.Pod{
				ID:      addr, // use address as stable ID for static mode
				Address: addr,
			})
		}
		r.SetPods(pods)
		metricsReg.RouterRingRebalancesTotal.Inc()
		metricsReg.RouterRingSize.Set(float64(len(pods)))
		slog.Info("ring seeded from static config", "pod_count", len(pods))
	}

	// Build the hint cache.
	hint := cache.New(10_000, cfg.HintCacheTTL)

	// Build the reverse-proxy handler (with metrics wired for routing decisions).
	h := &proxy.Handler{
		Extract:  extract.SessionID,
		Ring:     r,
		Hint:     hint,
		Fallback: proxy.NewRoundRobinFallback(r),
		Metrics:  metricsReg,
	}

	// Build the HTTP mux: route proxy traffic on / and expose /metrics.
	mux := http.NewServeMux()
	mux.Handle("/metrics", metricsReg.Handler())
	mux.Handle("/", h)

	// Wire up the HTTP server.
	srv := &http.Server{
		Addr:    cfg.Bind,
		Handler: mux,
		// Generous timeouts: the proxy handles streaming responses and WebSocket
		// upgrades, so we don't cut long-lived connections at the read/write
		// layer; the upstream portal pods enforce their own per-request timeouts.
		ReadHeaderTimeout: 10 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	// Context cancelled on SIGTERM or SIGINT.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, syscall.SIGINT)
	defer stop()

	listenErr := make(chan error, 1)
	go func() {
		listenErr <- srv.ListenAndServe()
	}()

	slog.Info("jamsesh-router listening", "bind", cfg.Bind)

	select {
	case err := <-listenErr:
		if errors.Is(err, http.ErrServerClosed) {
			// Normal shutdown path.
			return 0
		}
		slog.Error("jamsesh-router listen error", "err", err)
		return 1

	case <-ctx.Done():
		grace := time.Duration(cfg.ShutdownGraceSeconds) * time.Second
		slog.Info("jamsesh-router shutting down",
			"drain_budget_s", cfg.ShutdownGraceSeconds)

		shutCtx, cancel := context.WithTimeout(context.Background(), grace)
		defer cancel()

		if err := srv.Shutdown(shutCtx); err != nil {
			slog.Error("jamsesh-router shutdown error", "err", err)
			return 1
		}
		slog.Info("jamsesh-router stopped")
		return 0
	}
}

func printUsage() {
	fmt.Fprintf(os.Stderr, `jamsesh-router [config-file]

A consistent-hash reverse proxy for clustered jamsesh deployments.
Optional in single-instance mode.

Configuration may be provided via YAML file (first positional argument)
with environment variable overrides:

  JAMSESH_ROUTER_BIND               Listen address (default ":8080")
  JAMSESH_ROUTER_DISCOVERY_MODE     "static" | "kubernetes" (default "static")
  JAMSESH_ROUTER_STATIC_PODS        Comma-separated pod addresses (e.g. "10.0.0.1:8443,10.0.0.2:8443")
  JAMSESH_ROUTER_KUBE_NAMESPACE     Kubernetes namespace (kubernetes mode)
  JAMSESH_ROUTER_KUBE_SERVICE_NAME  Kubernetes service name (kubernetes mode)
  JAMSESH_ROUTER_SHUTDOWN_GRACE_S   Graceful drain budget in seconds (default 30)
`)
}
