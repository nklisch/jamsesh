// Package sessioncmd implements session-level subcommands for the jamsesh CLI:
// fork, mode (and join/status in the sibling story).
package sessioncmd

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"jamsesh/cmd/jamsesh/state"
)

// ResolveSession returns the jamsesh session ID for the current CC instance.
// It reads the CC_SESSION_ID environment variable (the Claude Code instance
// identifier) and maps it to the jamsesh session ID stored under
// ${data-dir}/sessions/<sessionID>/instance_id.
//
// If CC_SESSION_ID is not set, it falls back to reading the first session
// found in the sessions/ directory (useful for single-session development).
//
// Exported so sibling packages (e.g. cmd/jamsesh/finalizecmd) can reuse the
// same CC-session-id resolution without duplicating the mapping logic.
func ResolveSession() (string, error) {
	dir, err := state.DataDir()
	if err != nil {
		return "", err
	}

	ccID := os.Getenv("CC_SESSION_ID")

	sessionsDir := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return "", fmt.Errorf("no sessions found under %s; run `jamsesh join` first", sessionsDir)
		}
		return "", fmt.Errorf("reading sessions directory: %w", err)
	}

	// If we have a CC session ID, find the matching jamsesh session.
	if ccID != "" {
		for _, e := range entries {
			if !e.IsDir() {
				continue
			}
			instanceFile := filepath.Join(sessionsDir, e.Name(), "instance_id")
			data, err := os.ReadFile(instanceFile)
			if err != nil {
				continue
			}
			if strings.TrimSpace(string(data)) == ccID {
				return e.Name(), nil
			}
		}
		return "", fmt.Errorf("no session mapped to CC instance %q; run `jamsesh join` first", ccID)
	}

	// Fallback: use first directory entry (single-session dev path).
	for _, e := range entries {
		if e.IsDir() {
			return e.Name(), nil
		}
	}
	return "", fmt.Errorf("no sessions found under %s; run `jamsesh join` first", sessionsDir)
}


