// Package auth implements the "auth" subcommand for the jamsesh CLI.
// It provides two OAuth 2.0 flows — browser local-listener (default) and
// RFC 8628 device-code (--device-code) — both with PKCE (S256) and a state
// nonce for CSRF protection.
package auth

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"runtime"

	"github.com/urfave/cli/v3"

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

// WithOpenURL replaces the default browser-open function. Primarily used in
// tests to avoid launching a real browser.
func WithOpenURL(fn func(url string) error) Option {
	return func(c *config) { c.openURL = fn }
}

// defaultOpenURL opens url in the user's default browser using platform-
// appropriate mechanisms. We inline this helper rather than depending on
// github.com/pkg/browser to keep the dependency footprint minimal; the
// logic mirrors what that package does internally.
//
// On Linux we try xdg-open; on macOS we use open; on Windows we use
// rundll32. If the command is unavailable or fails, we print the URL so
// the user can open it manually — degrading gracefully.
func defaultOpenURL(rawURL string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		// Unknown platform — print the URL and let the user copy it.
		fmt.Fprintf(os.Stderr, "Cannot open browser automatically on %s.\nPlease visit: %s\n", runtime.GOOS, rawURL)
		return nil
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		// Non-fatal: print the URL so the user can open it manually.
		fmt.Fprintf(os.Stderr, "Could not launch browser: %v\nPlease visit: %s\n", err, rawURL)
		return nil
	}
	// Detach from the child — we don't wait for the browser to exit.
	go func() { _ = cmd.Wait() }()
	return nil
}
