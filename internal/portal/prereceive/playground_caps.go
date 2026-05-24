package prereceive

import (
	"context"
	"io/fs"
	"log/slog"
	"path/filepath"
)

// CodePlaygroundSizeExceeded is the push error code emitted when a playground
// session's accumulated content would exceed the configured cap.
const CodePlaygroundSizeExceeded = "playground.size_exceeded"

// reservedPlaygroundOrgID is the org_id for the playground org. We hard-code
// it here to avoid an import cycle (prereceive → playground would be cyclic);
// the value is also pinned as playground.ReservedOrgID.
const reservedPlaygroundOrgID = "org_playground"

// CheckPlaygroundCaps runs playground-specific pre-receive checks. It fires
// only when in.Session.OrgID matches the reserved playground org; all other
// session pushes return (Rejection{}, true) immediately so the performance
// impact on the durable-session path is a single string comparison.
//
// Currently enforced: per-session accumulated content-size cap (Option B).
// A push is rejected if the sum of the current on-disk repo size and the
// incoming pack size would exceed v.PlaygroundMaxContentBytes.
//
// The size measurement walks the on-disk repo directory. If the walk fails
// (e.g. the repo directory doesn't exist yet on the first push), the current
// size is treated as 0 and the check proceeds normally. Walk failures are
// logged as warnings; they don't fail the push.
//
// Returns (Rejection, false) when the cap is exceeded, (Rejection{}, true)
// when the push is allowed or the check is skipped.
func (v *Validator) CheckPlaygroundCaps(_ context.Context, in ValidateInput) (Rejection, bool) {
	if in.Session == nil || in.Session.OrgID != reservedPlaygroundOrgID {
		// Not a playground session — fast path out.
		return Rejection{}, true
	}
	if v.PlaygroundMaxContentBytes <= 0 {
		// Cap not configured — skip check.
		return Rejection{}, true
	}

	logger := v.Logger
	if logger == nil {
		logger = slog.Default()
	}

	// Measure the current on-disk repo size.
	currentBytes := repoDirSize(in.RepoPath, logger)

	// Reject if the incoming pack would push the total over the cap.
	if currentBytes+in.PackBytes > v.PlaygroundMaxContentBytes {
		return Rejection{
			Code: CodePlaygroundSizeExceeded,
			Message: "playground session content limit exceeded: this push would exceed the " +
				"maximum allowed repo size for a playground session",
			Details: map[string]any{
				"current_bytes":   currentBytes,
				"pack_bytes":      in.PackBytes,
				"total_bytes":     currentBytes + in.PackBytes,
				"max_bytes":       v.PlaygroundMaxContentBytes,
				"session_id":      in.Session.ID,
			},
		}, false
	}

	return Rejection{}, true
}

// repoDirSize walks the directory tree rooted at path and sums the sizes of
// all regular files. Returns 0 on any error (empty or non-existent repo is
// treated as 0 bytes so the first push into a fresh playground session is
// never rejected by the size check alone).
func repoDirSize(repoPath string, logger *slog.Logger) int64 {
	if repoPath == "" {
		return 0
	}
	var total int64
	err := filepath.WalkDir(repoPath, func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			// Directory entry errors (e.g. permission denied on a stale ref)
			// are skipped individually; don't abort the whole walk.
			return nil
		}
		if !d.IsDir() {
			info, infoErr := d.Info()
			if infoErr == nil {
				total += info.Size()
			}
		}
		return nil
	})
	if err != nil {
		logger.Warn("prereceive: playground repo-size walk failed; treating current size as 0",
			"repo_path", repoPath, "err", err)
		return 0
	}
	return total
}
