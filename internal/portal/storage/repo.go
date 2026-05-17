package storage

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
)

// CreateRepo initialises a new bare git repository on disk for the given
// org+session pair. It creates all necessary parent directories (0o750).
// Returns an error if the repo directory already exists.
func (s *service) CreateRepo(ctx context.Context, orgID, sessionID string) error {
	p := s.RepoPath(orgID, sessionID)

	// Ensure parent directories exist before checking/creating the repo dir.
	if err := os.MkdirAll(filepath.Dir(p), 0o750); err != nil {
		return fmt.Errorf("storage: mkdir parent: %w", err)
	}

	// Use os.Mkdir (not MkdirAll) so we get an error if the dir already exists.
	if err := os.Mkdir(p, 0o750); err != nil {
		if os.IsExist(err) {
			return fmt.Errorf("storage: repo already exists: %s", p)
		}
		return fmt.Errorf("storage: mkdir repo: %w", err)
	}

	// Initialise the bare repository inside the pre-created directory.
	// git init --bare <path> is idempotent on an empty dir but we own the
	// mkdir above, so we know it is fresh.
	cmd := exec.CommandContext(ctx, "git", "init", "--bare", p)
	cmd.Stderr = os.Stderr
	if out, err := cmd.Output(); err != nil {
		// Best-effort cleanup: remove the directory we just created so the
		// caller can retry without a stale directory.
		_ = os.RemoveAll(p)
		return fmt.Errorf("storage: git init --bare: %w (output: %s)", err, out)
	}

	return nil
}

// RemoveRepo hard-deletes the bare repo directory tree for the given
// org+session pair. If the path does not exist the call is a no-op (returns nil).
func (s *service) RemoveRepo(_ context.Context, orgID, sessionID string) error {
	return os.RemoveAll(s.RepoPath(orgID, sessionID))
}

// RepoExists reports whether a bare repo directory exists on disk for the
// given org+session pair. It returns (false, nil) when the path is absent and
// propagates unexpected stat errors.
func (s *service) RepoExists(orgID, sessionID string) (bool, error) {
	info, err := os.Stat(s.RepoPath(orgID, sessionID))
	if err != nil {
		if os.IsNotExist(err) {
			return false, nil
		}
		return false, err
	}
	return info.IsDir(), nil
}
