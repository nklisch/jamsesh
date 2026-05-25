// Package state provides read/write helpers for the Claude plugin's local
// data directory, rooted at ${CLAUDE_PLUGIN_DATA}. All credential files are
// written at mode 0600. All writes are atomic via temp-file + rename.
package state

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// DefaultPortalURL is the fallback portal URL when neither the env var nor
// the portal_url file is present. Points at the official hosted portal so
// out-of-the-box use works; self-hosters override via JAMSESH_PORTAL_URL
// or the portal_url state file. The /jamsesh:* skills still prompt for
// explicit configuration on first use so the choice is conscious.
const DefaultPortalURL = "https://jamsesh.dev"

// PluginDataDir returns the value of CLAUDE_PLUGIN_DATA, or an error if
// the environment variable is unset or empty.
func PluginDataDir() (string, error) {
	dir := os.Getenv("CLAUDE_PLUGIN_DATA")
	if dir == "" {
		return "", errors.New("CLAUDE_PLUGIN_DATA is not set; this binary must be invoked by the Claude Code plugin runtime")
	}
	return dir, nil
}

// Read returns the contents of <PluginDataDir>/<name>. Returns
// fs.ErrNotExist (unwrapped from the underlying os error) when the file
// does not exist so callers can use errors.Is(err, fs.ErrNotExist).
func Read(name string) ([]byte, error) {
	dir, err := PluginDataDir()
	if err != nil {
		return nil, err
	}
	data, err := os.ReadFile(filepath.Join(dir, name))
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, fmt.Errorf("state file %q not found: %w", name, fs.ErrNotExist)
		}
		return nil, fmt.Errorf("reading state file %q: %w", name, err)
	}
	return data, nil
}

// Write atomically writes data to <PluginDataDir>/<name> with the given
// mode. Atomicity is achieved by writing to a sibling temp file and then
// renaming it over the target. The temp file is always removed on failure.
func Write(name string, data []byte, mode fs.FileMode) error {
	dir, err := PluginDataDir()
	if err != nil {
		return err
	}
	target := filepath.Join(dir, name)

	// Write to a temp file in the same directory so the rename is atomic
	// (same filesystem).
	tmp, err := os.CreateTemp(dir, ".jamsesh-write-*")
	if err != nil {
		return fmt.Errorf("creating temp file for %q: %w", name, err)
	}
	tmpName := tmp.Name()

	// Ensure temp is cleaned up on any failure path.
	success := false
	defer func() {
		if !success {
			_ = os.Remove(tmpName)
		}
	}()

	if _, err := tmp.Write(data); err != nil {
		_ = tmp.Close()
		return fmt.Errorf("writing temp file for %q: %w", name, err)
	}
	if err := tmp.Close(); err != nil {
		return fmt.Errorf("closing temp file for %q: %w", name, err)
	}

	// Apply the requested permission before the rename so the target never
	// briefly appears with wrong permissions.
	if err := os.Chmod(tmpName, mode); err != nil {
		return fmt.Errorf("setting mode on temp file for %q: %w", name, err)
	}

	if err := os.Rename(tmpName, target); err != nil {
		return fmt.Errorf("renaming temp file to %q: %w", name, err)
	}
	success = true
	return nil
}

// ReadToken reads the "token" file and returns it with leading/trailing
// whitespace trimmed.
func ReadToken() (string, error) {
	data, err := Read("token")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteToken writes the access token to "token" at mode 0600.
func WriteToken(t string) error {
	return Write("token", []byte(t), 0o600)
}

// ReadRefreshToken reads the "refresh_token" file and returns it trimmed.
func ReadRefreshToken() (string, error) {
	data, err := Read("refresh_token")
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(data)), nil
}

// WriteRefreshToken writes the refresh token to "refresh_token" at mode 0600.
func WriteRefreshToken(t string) error {
	return Write("refresh_token", []byte(t), 0o600)
}

// ReadSessionToken returns the bearer token for the given session.
// Returns fs.ErrNotExist (via errors.Is) when no token exists for the session.
func ReadSessionToken(sessionID string) ([]byte, error) {
	return Read("sessions/" + sessionID + "/token")
}

// ReadCurrentBearer returns the most appropriate bearer token for the
// current binary invocation. Resolution order:
//
//  1. If sessionID is non-empty AND the per-session bearer file exists
//     AND is readable, return that token (trimmed).
//  2. Otherwise, fall back to ReadToken() (legacy account-wide path).
//
// Post-migration the legacy token is a `MIGRATED_TO_PER_SESSION` stub;
// callers that receive the stub should treat it as an absent token
// (the portal will reject it as a malformed bearer). Pre-migration or
// for unbound invocations (no sessionID) the legacy token is the
// canonical bearer.
//
// sessionID is typically resolved by the caller via the
// CLAUDE_SESSION_ID -> instance_id binding lookup; callers without a
// binding context pass "".
func ReadCurrentBearer(sessionID string) (string, error) {
	if sessionID != "" {
		if tok, err := ReadSessionToken(sessionID); err == nil {
			return strings.TrimSpace(string(tok)), nil
		}
		// Per-session miss is non-fatal; fall through to legacy.
	}
	return ReadToken()
}

// WriteSessionToken stores the bearer token for the given session at mode 0600.
// The sessions/<sessionID>/ directory is created if it does not already exist.
func WriteSessionToken(sessionID string, token []byte) error {
	dir, err := PluginDataDir()
	if err != nil {
		return err
	}
	sessDir := filepath.Join(dir, "sessions", sessionID)
	if err := os.MkdirAll(sessDir, 0o700); err != nil {
		return fmt.Errorf("creating session dir %q: %w", sessDir, err)
	}
	return Write("sessions/"+sessionID+"/token", token, 0o600)
}

// ListSessions returns the IDs of all sessions that have a subdirectory under
// ${CLAUDE_PLUGIN_DATA}/sessions/. The order is not guaranteed.
func ListSessions() ([]string, error) {
	dir, err := PluginDataDir()
	if err != nil {
		return nil, err
	}
	sessionsDir := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil, nil // no sessions directory yet — fresh install
		}
		return nil, fmt.Errorf("listing sessions dir: %w", err)
	}
	var ids []string
	for _, e := range entries {
		if e.IsDir() {
			ids = append(ids, e.Name())
		}
	}
	return ids, nil
}

// CurrentSessionID returns the jamsesh session ID bound to the current Claude
// Code instance, and true. It locates the binding by matching the
// CLAUDE_SESSION_ID environment variable against the instance_id files stored
// under ${CLAUDE_PLUGIN_DATA}/sessions/<sessionID>/instance_id.
//
// When CLAUDE_SESSION_ID is not set or no matching binding is found, it returns
// ("", false) — callers should omit the Jam-Session-Id header in that case.
func CurrentSessionID() (string, bool) {
	dir, err := PluginDataDir()
	if err != nil {
		return "", false
	}

	ccID := os.Getenv("CLAUDE_SESSION_ID")
	if ccID == "" {
		return "", false
	}

	sessionsDir := filepath.Join(dir, "sessions")
	entries, err := os.ReadDir(sessionsDir)
	if err != nil {
		return "", false
	}

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
			return e.Name(), true
		}
	}
	return "", false
}

// ReadPortalURL resolves the portal URL with the following precedence:
//  1. JAMSESH_PORTAL_URL environment variable
//  2. ${CLAUDE_PLUGIN_DATA}/portal_url file (trimmed)
//  3. DefaultPortalURL constant
func ReadPortalURL() (string, error) {
	if u := os.Getenv("JAMSESH_PORTAL_URL"); u != "" {
		return u, nil
	}
	data, err := Read("portal_url")
	if err == nil {
		if u := strings.TrimSpace(string(data)); u != "" {
			return u, nil
		}
	}
	// Ignore ErrNotExist or empty file — fall through to default.
	return DefaultPortalURL, nil
}
