package finalizecmd

import (
	"context"
	"io"

	"jamsesh/cmd/jamsesh/portalclient"
)

// fetchSource is the resolved local-or-https endpoint git fetch will
// pull from, plus a cleanup func registered with the orchestration
// layer. Story 2 replaces this scaffold with the real chooser
// (filesystem-state lookup + HTTPS fallback with ephemeral token).
type fetchSource struct {
	Kind    string // "local" | "https"
	URL     string // local path or HTTPS URL
	cleanup func() error
}

// chooseFetchSource is the placeholder this story ships. Story 2
// (epic-finalize-flow-plugin-finalize-command-fetch-source-selection-and-cleanup)
// replaces this with the local-first vs HTTPS-fallback chooser.
//
// Returning kind:"local" + URL:"." lets the rest of the flow compile
// and the unit tests of every other step run without a portal touch.
// The real cherry-pick still works against the user's checkout since
// git fetch against "." is effectively a no-op against the same repo.
func chooseFetchSource(ctx context.Context, pc *portalclient.Client, plan *Plan, sessionID string) (*fetchSource, error) {
	return &fetchSource{
		Kind:    "local",
		URL:     ".",
		cleanup: func() error { return nil },
	}, nil
}

// performFetch runs `git fetch <source.URL>` with verbose logging.
// In stub form this fetches from "." (no-op against the current
// repo); story 2 replaces with a real `git remote add jamsesh ...`
// + `git fetch jamsesh` cycle.
func performFetch(out io.Writer, fs *fetchSource) error {
	return runGitVerbose(out, "fetch", fs.URL)
}
