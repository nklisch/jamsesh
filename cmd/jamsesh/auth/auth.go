// Package auth implements the "auth" subcommand for the jamsesh CLI.
// It provides two OAuth 2.0 flows — browser local-listener (default) and
// RFC 8628 device-code (--device-code) — both with PKCE (S256) and a state
// nonce for CSRF protection.
package auth

import (
	"context"
	"fmt"
	"os"

	"github.com/urfave/cli/v3"

	"jamsesh/cmd/jamsesh/internal/osopen"
	"jamsesh/cmd/jamsesh/state"
)

// Command returns the urfave/cli command descriptor for "jamsesh auth".
// Functional options allow tests to inject dependencies (e.g. a no-op
// browser opener) without touching production code paths.
func Command(opts ...Option) *cli.Command {
	cfg := &config{
		openURL: defaultOpenURL,
	}
	for _, o := range opts {
		o(cfg)
	}

	return &cli.Command{
		Name:  "auth",
		Usage: "Authenticate with the jamsesh portal (OAuth)",
		Flags: []cli.Flag{
			&cli.BoolFlag{
				Name:  "device-code",
				Usage: "Use the RFC 8628 device authorization grant instead of the browser flow",
			},
		},
		Action: func(ctx context.Context, cmd *cli.Command) error {
			portalURL, err := state.ReadPortalURL()
			if err != nil {
				return fmt.Errorf("resolving portal URL: %w", err)
			}

			if cmd.Bool("device-code") {
				return deviceFlow(ctx, portalURL, nil)
			}
			return browserFlow(ctx, portalURL, cfg.openURL)
		},
	}
}

// config holds injectable dependencies for the auth command.
type config struct {
	openURL func(url string) error
}

// Option is a functional option for Command.
type Option func(*config)

// defaultOpenURL opens rawURL in the user's default browser. It delegates to
// osopen.Open for the platform-specific launcher logic with graceful
// degradation.
func defaultOpenURL(rawURL string) error {
	return osopen.Open(rawURL, os.Stderr)
}
