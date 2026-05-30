package finalizecmd

import (
	"os"

	"jamsesh/cmd/jamsesh/internal/osopen"
)

// openURL is the function var the finalize subcommand calls; tests
// override it to avoid launching a real browser.
var openURL = func(rawURL string) error {
	return osopen.Open(rawURL, os.Stderr)
}
