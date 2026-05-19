package githttp

import (
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"regexp"
	"strings"

	"github.com/go-chi/chi/v5"

	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/httperr"
)

// gitProtocolRE validates the Git-Protocol header value before propagating it
// to the subprocess environment. Matches Gitea's allowlist to prevent env-var
// injection.
var gitProtocolRE = regexp.MustCompile(`^[0-9a-zA-Z:=_.-]+$`)

// infoRefs handles GET /{orgID}/{sessionID}.git/info/refs?service=...
//
// It writes the smart-HTTP service advertisement: a pkt-line service header
// followed by the output of `git <svc> --stateless-rpc --advertise-refs`.
func (h *Handler) infoRefs(w http.ResponseWriter, r *http.Request) {
	service := r.URL.Query().Get("service")
	if service != "git-upload-pack" && service != "git-receive-pack" {
		http.Error(w, "invalid service", http.StatusBadRequest)
		return
	}

	orgID := chi.URLParam(r, "orgID")
	sessionID := chi.URLParam(r, "sessionID")

	// Clustered mode: hydrate the bare repo from object storage before serving.
	// In single-instance mode (h.Lifecycle == nil) this is a no-op.
	if err := h.acquireForGitRequest(r.Context(), sessionID); err != nil {
		httperr.Write(w, r, httperr.ErrObjectStorageUnavailable(err))
		return
	}

	repoPath := h.Storage.RepoPath(orgID, sessionID)

	// Strip "git-" prefix to get the git subcommand name.
	svc := strings.TrimPrefix(service, "git-")

	cmd := exec.CommandContext(r.Context(),
		"git", svc, "--stateless-rpc", "--advertise-refs", repoPath)
	cmd.Env = append(cmd.Environ(), "GIT_DIR="+repoPath)
	if v := r.Header.Get("Git-Protocol"); v != "" && gitProtocolRE.MatchString(v) {
		cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
	}

	out, err := cmd.Output()
	if err != nil {
		slog.ErrorContext(r.Context(), "info/refs subprocess failed",
			"err", err, "service", service, "repo", repoPath)
		httperr.Write(w, r,
			httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
		return
	}

	w.Header().Set("Content-Type", "application/x-"+service+"-advertisement")
	w.Header().Set("Cache-Control", "no-cache")
	w.WriteHeader(http.StatusOK)

	// Write the smart-HTTP service announcement pkt-line then the flush packet.
	// Format: <4-hex-len># service=<service>\n0000
	// The 4-hex-len includes the 4-byte length prefix itself.
	prefix := fmt.Sprintf("# service=%s\n", service)
	fmt.Fprintf(w, "%04x%s0000", len(prefix)+4, prefix)

	_, _ = w.Write(out)
}
