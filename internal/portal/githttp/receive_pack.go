package githttp

import (
	"bytes"
	"io"
	"log/slog"
	"net/http"
	"os"
	"os/exec"
	"strings"

	"github.com/go-chi/chi/v5"
	git "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/format/packfile"
	"github.com/go-git/go-git/v5/plumbing/storer"
	gogitstorage "github.com/go-git/go-git/v5/storage"
	"github.com/go-git/go-git/v5/storage/memory"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/gitref"
	"jamsesh/internal/portal/httperr"
	"jamsesh/internal/portal/prereceive"
)

// retryAfterSeconds is the Retry-After value sent with 503 responses when
// the per-instance receive-pack concurrency semaphore is full.
const retryAfterSeconds = "5"

// receivePack handles POST /{orgID}/{sessionID}.git/git-receive-pack.
//
// Flow:
//  1. Acquire per-instance concurrency semaphore (503 if full).
//  2. Stream body to a tempfile (capped at MaxPackBytes + command-list overhead).
//  3. Parse the ref-update command list from the pkt-line prefix.
//  4. Run pre-receive validation against a layered repo (memory pack + disk).
//  5. On rejection: write the smart-HTTP report-status response and return.
//  6. On acceptance: rewind tempfile, spawn `git receive-pack --stateless-rpc`,
//     pipe the full body to stdin, stream stdout to the client.
//  7. On receive-pack exit 0: emit post-receive events.
func (h *Handler) receivePack(w http.ResponseWriter, r *http.Request) {
	// Acquire concurrency semaphore. If the semaphore is full, reject immediately
	// with 503 so the git client can retry rather than holding an HTTP connection
	// open indefinitely (which would cascade under high concurrency).
	if h.ReceivePackSem != nil {
		select {
		case h.ReceivePackSem <- struct{}{}:
			defer func() { <-h.ReceivePackSem }()
		default:
			w.Header().Set("Retry-After", retryAfterSeconds)
			http.Error(w, "too many concurrent pushes, retry shortly", http.StatusServiceUnavailable)
			return
		}
	}

	orgID := chi.URLParam(r, "orgID")
	sessionID := chi.URLParam(r, "sessionID")

	account, ok := AccountFromContext(r.Context())
	if !ok {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}

	// Enforce content-type.
	if r.Header.Get("Content-Type") != "application/x-git-receive-pack-request" {
		http.Error(w, "bad content type", http.StatusBadRequest)
		return
	}

	// Cap the request body. Add 16 KiB of slop for the command list overhead.
	maxBytes := h.Validator.MaxPackBytes + 16*1024
	if maxBytes <= 0 {
		// Validator has no limit configured — use a generous default.
		maxBytes = 128 * 1024 * 1024
	}
	limitedBody := http.MaxBytesReader(w, r.Body, maxBytes)

	// Stream the body to a tempfile rather than io.ReadAll into memory.
	// This keeps RSS bounded to a single copy of the pack on disk;
	// concurrent pushes no longer saturate RSS by holding N × 2× the
	// pack in RAM (once as bytes, once in the memory.Storage during
	// buildValidationRepo).
	bodyFile, err := os.CreateTemp("", "jamsesh-pack-*")
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: create tempfile", "err", err)
		httperr.Write(w, r, httperr.ErrInternal(err))
		return
	}
	defer func() {
		_ = bodyFile.Close()
		_ = os.Remove(bodyFile.Name())
	}()

	bodySize, err := io.Copy(bodyFile, limitedBody)
	if err != nil {
		// MaxBytesReader returns a specific error when the cap is exceeded;
		// treat any error as "too large" since that is the only likely failure.
		http.Error(w, "pack exceeds size limit", http.StatusRequestEntityTooLarge)
		return
	}

	// Rewind to the beginning for the command-list parse pass.
	if _, err := bodyFile.Seek(0, io.SeekStart); err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: seek tempfile", "err", err)
		httperr.Write(w, r, httperr.ErrInternal(err))
		return
	}

	// Parse the command list from the beginning of the body.
	updates, caps, packReader, err := readCommandList(bodyFile)
	if err != nil {
		http.Error(w, "malformed push request", http.StatusBadRequest)
		return
	}

	// Clustered mode: hydrate the bare repo from object storage before serving.
	// In single-instance mode (h.Lifecycle == nil) this is a no-op. This must
	// run after auth and body parsing to avoid hydrating for unauthenticated or
	// malformed requests, but before RepoPath is used to open the bare repo.
	if err := h.acquireForGitRequest(r.Context(), sessionID); err != nil {
		httperr.Write(w, r, httperr.ErrObjectStorageUnavailable(err))
		return
	}

	// Build a validation repo: parse the pushed pack into memory storage, then
	// layer it over the existing disk repo so the prereceive validator can see
	// both new objects (in the pack) and existing objects (on disk).
	repoPath := h.Storage.RepoPath(orgID, sessionID)
	validationRepo, err := buildValidationRepo(repoPath, packReader)
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: build validation repo",
			"err", err, "repo", repoPath)
		httperr.Write(w, r, httperr.ErrInternal(err))
		return
	}

	// Look up the session for validation context.
	session, err := h.Store.GetSession(r.Context(), orgID, sessionID)
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: get session",
			"err", err, "org", orgID, "session", sessionID)
		httperr.WriteFromError(w, r, deperr.WrapDBIfTransient(err))
		return
	}

	// Run pre-receive policy checks.
	validationIn := prereceive.ValidateInput{
		Repo:      validationRepo,
		Session:   &session,
		Account:   account,
		Updates:   updates,
		PackBytes: bodySize,
	}
	result, err := h.Validator.Validate(r.Context(), validationIn)
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: validate",
			"err", err, "org", orgID, "session", sessionID)
		httperr.Write(w, r, httperr.ErrInternal(err))
		return
	}

	// Set headers before writing any body.
	w.Header().Set("Content-Type", "application/x-git-receive-pack-result")
	w.Header().Set("Cache-Control", "no-cache")

	if !result.OK {
		// Pre-receive rejected: synthesise the report-status response so the
		// git client displays the rejection messages inline.
		writeReportStatusRejection(w, updates, result.Rejections, caps)
		if h.Metrics != nil {
			h.Metrics.GitPushesTotal.WithLabelValues("rejected").Inc()
		}
		return
	}

	// Rewind the tempfile to the start so the subprocess receives the full
	// body (command list + pack), not just the pack portion.
	if _, err := bodyFile.Seek(0, io.SeekStart); err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: seek tempfile for subprocess", "err", err)
		httperr.Write(w, r, httperr.ErrInternal(err))
		return
	}

	// Spawn receive-pack subprocess; feed the full body (command list + pack).
	cmd := exec.CommandContext(r.Context(),
		"git", "receive-pack", "--stateless-rpc", repoPath)
	cmd.Env = append(cmd.Environ(), "GIT_DIR="+repoPath)
	if v := r.Header.Get("Git-Protocol"); v != "" && gitProtocolRE.MatchString(v) {
		cmd.Env = append(cmd.Env, "GIT_PROTOCOL="+v)
	}

	stdin, err := cmd.StdinPipe()
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: stdin pipe", "err", err)
		httperr.Write(w, r,
			httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
		return
	}
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: stdout pipe", "err", err)
		httperr.Write(w, r,
			httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
		return
	}
	cmd.Stderr = os.Stderr

	if err := cmd.Start(); err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: start subprocess", "err", err)
		httperr.Write(w, r,
			httperr.ErrGitSubprocessFailed(deperr.WrapGitSubprocess(err)))
		return
	}

	// Write full body from the tempfile to subprocess stdin in a goroutine to
	// avoid deadlock while simultaneously draining subprocess stdout into a buffer.
	//
	// We buffer stdout rather than streaming it directly to the client so that
	// the object-storage sync (EmitForUpdates) can run before any response bytes
	// are committed. This is the RPO=0 contract: the git client must not receive
	// a success acknowledgement until the pushed objects are durable in object
	// storage. If the sync fails we can still return HTTP 500 because headers
	// have not yet been written.
	stdinErrCh := make(chan error, 1)
	go func() {
		defer stdin.Close()
		_, err := io.Copy(stdin, bodyFile)
		stdinErrCh <- err
	}()

	var subprocOut bytes.Buffer
	io.Copy(&subprocOut, stdout) //nolint:errcheck // read until EOF; errors manifest as truncation

	// Wait for subprocess to exit.
	cmdErr := cmd.Wait()
	<-stdinErrCh // drain stdin goroutine

	if cmdErr != nil {
		slog.ErrorContext(r.Context(), "receive-pack subprocess exited non-zero",
			"err", cmdErr, "org", orgID, "session", sessionID)
		// Subprocess already wrote its own error report into subprocOut. Flush
		// it to the client now — headers are not yet committed so we can set 200
		// (the report-status payload is what git parses for the error detail).
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(subprocOut.Bytes())
		return
	}

	// Open the on-disk repo for post-receive event emission. The pushed objects
	// are now committed to disk so go-git can walk them normally.
	diskRepo, err := git.PlainOpen(repoPath)
	if err != nil {
		slog.ErrorContext(r.Context(), "receive-pack: open repo post-push",
			"err", err, "repo", repoPath)
		// Disk is readable but we can't open it — treat as internal error.
		// Headers are not yet written so we can return 500.
		httperr.Write(w, r, httperr.ErrInternal(err))
		return
	}

	// Post-receive: bootstrap the draft ref from the base ref on first push.
	// The base ref (refs/heads/jam/<session>/base) is the only ref that is
	// allowed when the repo is empty. When it lands, we create the draft ref
	// (refs/heads/jam/<session>/draft) pointing at the same commit so that the
	// auto-merger has a starting point for all subsequent sync-ref merges.
	baseRefName := plumbing.NewBranchReferenceName("jam/" + sessionID + "/base")
	draftRefName := plumbing.NewBranchReferenceName("jam/" + sessionID + "/draft")
	for _, u := range updates {
		if u.Ref == baseRefName.String() && u.OldSHA == "" {
			// This is the base ref's first push. Seed the draft ref if not present.
			if _, err := diskRepo.Reference(draftRefName, true); err != nil {
				hash := plumbing.NewHash(u.NewSHA)
				draftRef := plumbing.NewHashReference(draftRefName, hash)
				if setErr := diskRepo.Storer.SetReference(draftRef); setErr != nil {
					slog.ErrorContext(r.Context(), "receive-pack: seed draft ref from base",
						"err", setErr, "org", orgID, "session", sessionID)
				}
			}
			break
		}
	}

	// Post-receive: stamp sessions.base_sha when the base ref lands for the
	// first time. findBaseRefUpdate returns non-nil only for a push that creates
	// refs/heads/jam/<sessionID>/base (exactly the two-segment form; user refs
	// like refs/heads/jam/<id>/<user>/base are not matched).
	if baseUpdate := findBaseRefUpdate(sessionID, updates); baseUpdate != nil {
		sha := baseUpdate.NewSHA
		if err := h.Store.SetSessionBaseSHA(r.Context(), store.SetSessionBaseSHAParams{
			OrgID:   orgID,
			ID:      sessionID,
			BaseSHA: &sha,
		}); err != nil {
			// Non-fatal: log but don't fail the push.
			// Worst case is the audit field stays NULL; doesn't affect runtime.
			slog.WarnContext(r.Context(), "receive-pack: set base_sha failed",
				"session_id", sessionID, "err", err)
		}
	}

	// Post-receive: emit commit.arrived events for accepted ref updates.
	// In clustered mode the Emitter also calls SyncPushPath, which mirrors the
	// push to object storage. We MUST call this BEFORE committing 200 OK —
	// if the sync fails (e.g. NoSuchBucket), we return 500 so the git client
	// sees a non-zero exit and does not believe the push succeeded.
	if h.Emitter != nil {
		if emitErr := h.Emitter.EmitForUpdates(
			r.Context(), diskRepo, &session, account,
			updates,
		); emitErr != nil {
			slog.ErrorContext(r.Context(), "receive-pack: post-receive emit (object-storage sync failure)",
				"err", emitErr, "org", orgID, "session", sessionID)
			// Headers are not yet committed — return 500 so the git client sees
			// a non-2xx response and exits non-zero. This preserves the RPO=0
			// contract: a push is never acknowledged when its objects are not
			// durable in object storage.
			httperr.Write(w, r, httperr.ErrInternal(emitErr))
			if h.Metrics != nil {
				h.Metrics.GitPushesTotal.WithLabelValues("storage_error").Inc()
			}
			return
		}
	}

	// Object-storage sync succeeded (or is not configured in single-instance
	// mode). Now commit 200 OK and flush the buffered subprocess output.
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(subprocOut.Bytes())

	// Record successful push outcome. This runs after the subprocess exits 0 and
	// post-receive events are dispatched, so it reflects end-to-end success.
	if h.Metrics != nil {
		h.Metrics.GitPushesTotal.WithLabelValues("ok").Inc()
	}
}

// findBaseRefUpdate scans updates for the one that creates the session's base
// ref: refs/heads/jam/<sessionID>/base. It matches only the exact two-segment
// form after "refs/heads/jam/" (i.e. "<sessionID>/base"), so user refs like
// refs/heads/jam/<id>/<accountID>/base are never mistakenly matched.
//
// Returns nil when no base-ref update is found. Pre-receive guarantees at most
// one base-ref push per receive (once refs exist, pushing to base is rejected),
// so at most one match is expected.
func findBaseRefUpdate(sessionID string, updates []gitref.RefUpdate) *gitref.RefUpdate {
	prefix := "refs/heads/jam/"
	want := prefix + sessionID + "/base"
	for i := range updates {
		u := &updates[i]
		if u.Ref != want {
			continue
		}
		// Extra guard: ensure this is exactly the two-segment form by verifying
		// there is no further slash after the expected suffix. Because we match
		// the full string above this is already guaranteed, but make the intent
		// explicit with a strings.Count check.
		rest := strings.TrimPrefix(u.Ref, prefix)
		if strings.Count(rest, "/") != 1 {
			// Shouldn't happen given the exact-string match above, but be safe.
			continue
		}
		return u
	}
	return nil
}

// buildValidationRepo creates a *git.Repository suitable for pre-receive
// validation. It parses the pushed pack into an in-memory storer and layers
// it over the existing bare repo so the prereceive validator can see both
// new objects (in the pack) and existing objects (on disk).
//
// This is necessary because git-receive-pack quarantines incoming objects in
// a temp directory that go-git's dotgit storage cannot see
// (src-d/go-git#886). By parsing the pack ourselves we bypass the quarantine.
func buildValidationRepo(repoPath string, packData io.Reader) (*git.Repository, error) {
	// 1. Parse the pushed pack into memory storage.
	memStore := memory.NewStorage()
	packBytes, err := io.ReadAll(packData)
	if err != nil {
		return nil, err
	}

	if len(packBytes) > 0 {
		scanner := packfile.NewScanner(bytes.NewReader(packBytes))
		parser, err := packfile.NewParserWithStorage(scanner, memStore)
		if err != nil {
			return nil, err
		}
		if _, err := parser.Parse(); err != nil {
			return nil, err
		}
	}

	// 2. Open the on-disk bare repo for fallback object lookups.
	diskRepo, err := git.PlainOpen(repoPath)
	if err != nil {
		return nil, err
	}

	// 3. Build a layered store: objects from memory first, then disk.
	ls := &layeredStorer{
		Storage:      memStore,
		diskObjects:  diskRepo.Storer,
		diskRefs:     diskRepo.Storer,
	}
	return git.Open(ls, nil)
}

// layeredStorer embeds the in-memory Storage to satisfy storage.Storer, but
// overrides object resolution to fall through to the on-disk repo when an
// object is not found in memory. References come from the disk repo.
//
// This allows prereceive validation to walk new commits (from the pack,
// which are in memory) and resolve existing commits (from the bare repo) for
// force-push ancestor checks.
type layeredStorer struct {
	*memory.Storage            // provides all non-object methods (Config, Index, Shallow, Module, etc.)
	diskObjects gogitstorage.Storer
	diskRefs    gogitstorage.Storer
}

// EncodedObject tries memory first; falls through to disk on miss.
func (s *layeredStorer) EncodedObject(t plumbing.ObjectType, h plumbing.Hash) (plumbing.EncodedObject, error) {
	obj, err := s.Storage.EncodedObject(t, h)
	if err == plumbing.ErrObjectNotFound {
		return s.diskObjects.EncodedObject(t, h)
	}
	return obj, err
}

// HasEncodedObject checks memory first, then disk.
func (s *layeredStorer) HasEncodedObject(h plumbing.Hash) error {
	if err := s.Storage.HasEncodedObject(h); err == nil {
		return nil
	}
	return s.diskObjects.HasEncodedObject(h)
}

// EncodedObjectSize checks memory first, then disk.
func (s *layeredStorer) EncodedObjectSize(h plumbing.Hash) (int64, error) {
	n, err := s.Storage.EncodedObjectSize(h)
	if err == plumbing.ErrObjectNotFound {
		return s.diskObjects.EncodedObjectSize(h)
	}
	return n, err
}

// IterEncodedObjects returns objects from memory and disk combined.
func (s *layeredStorer) IterEncodedObjects(t plumbing.ObjectType) (storer.EncodedObjectIter, error) {
	memIter, err := s.Storage.IterEncodedObjects(t)
	if err != nil {
		return nil, err
	}
	diskIter, err := s.diskObjects.IterEncodedObjects(t)
	if err != nil {
		return nil, err
	}
	return storer.NewMultiEncodedObjectIter([]storer.EncodedObjectIter{memIter, diskIter}), nil
}

// Reference delegates to the disk repo (new refs haven't landed yet).
func (s *layeredStorer) Reference(n plumbing.ReferenceName) (*plumbing.Reference, error) {
	return s.diskRefs.Reference(n)
}

// IterReferences delegates to the disk repo.
func (s *layeredStorer) IterReferences() (storer.ReferenceIter, error) {
	return s.diskRefs.IterReferences()
}

// SetReference delegates to the disk repo (used by git.Open internally).
func (s *layeredStorer) SetReference(r *plumbing.Reference) error {
	return s.diskRefs.SetReference(r)
}

// CheckAndSetReference delegates to the disk repo.
func (s *layeredStorer) CheckAndSetReference(new, old *plumbing.Reference) error {
	return s.diskRefs.CheckAndSetReference(new, old)
}

// RemoveReference delegates to the disk repo.
func (s *layeredStorer) RemoveReference(n plumbing.ReferenceName) error {
	return s.diskRefs.RemoveReference(n)
}

// CountLooseRefs delegates to the disk repo.
func (s *layeredStorer) CountLooseRefs() (int, error) {
	return s.diskRefs.CountLooseRefs()
}

// PackRefs delegates to the disk repo.
func (s *layeredStorer) PackRefs() error {
	return s.diskRefs.PackRefs()
}

