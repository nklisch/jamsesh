// Package server provides the portal HTTP server lifecycle.
// Run blocks until ctx is cancelled (graceful shutdown) or a listen error
// occurs. It handles both native TLS and proxied HTTP modes.
package server

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"time"

	"jamsesh/internal/portal/config"
)

// Run starts the HTTP server on cfg.Bind with handler and blocks until
// ctx is cancelled or a fatal listen error occurs.
//
// TLS modes:
//   - "native": ListenAndServeTLS using cfg.TLS.CertPath and cfg.TLS.KeyPath
//   - "behind_proxy" (default): plain HTTP, TLS terminated upstream
//
// On ctx cancellation, Shutdown is called with a 25-second drain budget.
// Returns nil on graceful shutdown, error on listen failure or shutdown
// timeout (callers map non-nil to a non-zero exit code).
func Run(ctx context.Context, cfg config.Config, handler http.Handler) error {
	srv := &http.Server{
		Addr:              cfg.Bind,
		Handler:           handler,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       30 * time.Second,
		IdleTimeout:       2 * time.Minute,
	}

	listenErr := make(chan error, 1)
	go func() {
		switch cfg.TLS.Mode {
		case "native":
			listenErr <- srv.ListenAndServeTLS(cfg.TLS.CertPath, cfg.TLS.KeyPath)
		default: // "behind_proxy"
			listenErr <- srv.ListenAndServe()
		}
	}()

	slog.InfoContext(ctx, "portal listening",
		"bind", cfg.Bind,
		"tls_mode", cfg.TLS.Mode,
	)

	select {
	case err := <-listenErr:
		// ListenAndServe returns ErrServerClosed after Shutdown; treat as
		// clean exit. Any other error (e.g. address already in use) is fatal.
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err

	case <-ctx.Done():
		slog.InfoContext(context.Background(), "portal shutting down",
			"drain_budget_s", 25)
		shutCtx, cancel := context.WithTimeout(context.Background(), 25*time.Second)
		defer cancel()
		if err := srv.Shutdown(shutCtx); err != nil {
			return err
		}
		return nil
	}
}
