package githttp

import (
	"log/slog"
	"net/http"
	"os/exec"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/httperr"
)

// uploadPack handles POST /{orgID}/{sessionID}.git/git-upload-pack
//
// It pipes the request body to `git upload-pack --stateless-rpc` and streams
// the subprocess stdout back to the client using http.Flusher to keep large
// fetches alive without buffering the entire packfile in memory.
func (h *Handler) uploadPack(w http.ResponseWriter, r *http.Request) {
	orgID := chi.URLParam(r, "orgID")
	sessionID := chi.URLParam(r, "sessionID")
	repoPath := h.Storage.RepoPath(orgID, sessionID)

	cmd := exec.CommandContext(r.Context(),
		"git", "upload-pack", "--stateless-rpc", repoPath)
	cmd.Env = append(cmd.Environ(), "GIT_DIR="+repoPath)
	if v := r.Header.Get("Git-Protocol"); v != "" && gitProtocolRE.MatchString(v) {
		cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
	}

	cmd.Stdin = r.Body

	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.ErrorContext(r.Context(), "upload-pack stdout pipe", "err", err)
		httperr.Write(w, r,
			httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
		return
	}

	if err := cmd.Start(); err != nil {
		slog.ErrorContext(r.Context(), "upload-pack start", "err", err, "repo", repoPath)
		httperr.Write(w, r,
			httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
		return
	}

	w.Header().Set("Content-Type", "application/x-git-upload-pack-result")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	streamWithFlush(w, stdout)

	if err := cmd.Wait(); err != nil {
		// Response headers are already sent; we can only log the error.
		slog.ErrorContext(r.Context(), "upload-pack subprocess exited non-zero",
			"err", err, "repo", repoPath)
	}
}


