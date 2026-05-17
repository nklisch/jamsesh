package finalizecmd

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"runtime"
)

// defaultOpenURL opens rawURL in the user's default browser using
// platform-appropriate mechanisms. Inlined here rather than imported
// from cmd/jamsesh/auth/auth.go to avoid a cross-package coupling for
// what is effectively 10 lines of code. If a third consumer appears,
// promote to cmd/jamsesh/internal/osopen.
//
// On Linux we try xdg-open; on macOS we use open; on Windows we use
// rundll32. Any failure degrades gracefully: we write the URL to errOut
// so the user can copy it manually and return nil.
func defaultOpenURL(rawURL string, errOut io.Writer) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "linux":
		cmd = exec.Command("xdg-open", rawURL)
	case "darwin":
		cmd = exec.Command("open", rawURL)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", rawURL)
	default:
		fmt.Fprintf(errOut, "Cannot open browser automatically on %s.\nPlease visit: %s\n", runtime.GOOS, rawURL)
		return nil
	}
	cmd.Stdout = nil
	cmd.Stderr = nil
	if err := cmd.Start(); err != nil {
		fmt.Fprintf(errOut, "Could not launch browser: %v\nPlease visit: %s\n", err, rawURL)
		return nil
	}
	// Detach — don't wait for the browser to exit.
	go func() { _ = cmd.Wait() }()
	return nil
}

// openURL is the function var the finalize subcommand calls; tests
// override it to avoid launching a real browser.
var openURL = func(rawURL string) error {
	return defaultOpenURL(rawURL, os.Stderr)
}
