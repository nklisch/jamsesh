package finalize

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"time"

	gogit "github.com/go-git/go-git/v5"
	"github.com/go-git/go-git/v5/plumbing"
	"github.com/go-git/go-git/v5/plumbing/object"

	"jamsesh/internal/api/openapi"
	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/deperr"
	"jamsesh/internal/portal/tokens"
)

// GetFinalizePlan implements GET
// /api/orgs/{orgID}/sessions/{sessionID}/finalize-plan?lock_id=...
//
// Returns a deterministic plan computed from the lock's curation state +
// the live bare-repo commit metadata. Same lock state + same bare repo →
// same script bytes.
//
// Conflict semantics (409 ErrorEnvelope.error codes):
//
//   - finalize.lock_expired       — last_activity_at is older than the TTL
//   - finalize.lock_superseded    — the lock has been overridden by another
//   - finalize.commit_missing     — a curated SHA is absent from the repo;
//                                   details.missing_sha is the SHA
//
// 404 is returned when the lock is unknown or belongs to a different
// session (we don't leak existence on other sessions). 403 from the
// membership check. 401 when there is no caller in context.
func (h *Handler) GetFinalizePlan(ctx context.Context, req openapi.GetFinalizePlanRequestObject) (openapi.GetFinalizePlanResponseObject, error) {
	acc, ok := tokens.AccountFromContext(ctx)
	if !ok {
		return openapi.GetFinalizePlan401JSONResponse{
			UnauthorizedJSONResponse: openapi.UnauthorizedJSONResponse{
				Error:   "auth.invalid_token",
				Message: "invalid token",
			},
		}, nil
	}

	orgID := req.OrgID
	sessionID := req.SessionID
	lockID := req.Params.LockId

	verdict, err := checkSessionMembership(ctx, h.store, orgID, sessionID, acc.ID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: membership check: %w", err))
	}
	switch verdict {
	case memberNotOrgMember:
		return openapi.GetFinalizePlan403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this org",
			},
		}, nil
	case memberNotSessionMember:
		return openapi.GetFinalizePlan403JSONResponse{
			ForbiddenJSONResponse: openapi.ForbiddenJSONResponse{
				Error:   "auth.insufficient_permission",
				Message: "not a member of this session",
			},
		}, nil
	case memberSessionNotFound:
		return openapi.GetFinalizePlan404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "session.not_found",
				Message: "session not found",
			},
		}, nil
	}

	lock, err := h.store.GetFinalizeLockByID(ctx, lockID)
	if err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return openapi.GetFinalizePlan404JSONResponse{
				NotFoundJSONResponse: openapi.NotFoundJSONResponse{
					Error:   "finalize.lock_not_found",
					Message: "finalize lock not found",
				},
			}, nil
		}
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get lock: %w", err))
	}

	// Cross-session id mismatch ⇒ 404 (don't leak existence on other sessions).
	if lock.SessionID != sessionID || lock.OrgID != orgID {
		return openapi.GetFinalizePlan404JSONResponse{
			NotFoundJSONResponse: openapi.NotFoundJSONResponse{
				Error:   "finalize.lock_not_found",
				Message: "finalize lock not found",
			},
		}, nil
	}

	// Superseded ⇒ 409 with details.superseded_by_lock_id.
	if lock.SupersededByLockID != nil {
		return openapi.GetFinalizePlan409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.lock_superseded",
			Message: "finalize lock has been superseded",
			Details: map[string]interface{}{
				"superseded_by_lock_id": *lock.SupersededByLockID,
			},
		}), nil
	}

	now := time.Now().UTC()
	if IsLockExpired(lock.LastActivityAt, now) {
		return openapi.GetFinalizePlan409JSONResponse(openapi.ErrorEnvelope{
			Error:   "finalize.lock_expired",
			Message: "finalize lock idle for more than 30 minutes",
		}), nil
	}

	// Parse the curated SHA list.
	var shas []string
	if lock.SelectedCommitSHAs != "" {
		if err := json.Unmarshal([]byte(lock.SelectedCommitSHAs), &shas); err != nil {
			return nil, fmt.Errorf("finalize: unmarshal selected_commit_shas: %w", err)
		}
	}

	// Open the bare repo and resolve each curated SHA.
	repoPath := h.storage.RepoPath(orgID, sessionID)
	repo, err := gogit.PlainOpen(repoPath)
	if err != nil {
		return nil, fmt.Errorf("finalize: open repo %s: %w", repoPath, err)
	}

	commits := make([]*object.Commit, 0, len(shas))
	for _, sha := range shas {
		c, err := repo.CommitObject(plumbing.NewHash(sha))
		if err != nil {
			if errors.Is(err, plumbing.ErrObjectNotFound) {
				return openapi.GetFinalizePlan409JSONResponse(openapi.ErrorEnvelope{
					Error:   "finalize.commit_missing",
					Message: "a curated commit is no longer present in the session repo",
					Details: map[string]interface{}{
						"missing_sha": sha,
					},
				}), nil
			}
			return nil, fmt.Errorf("finalize: resolve commit %s: %w", sha, err)
		}
		commits = append(commits, c)
	}

	// Look up the session goal for default-subject derivation.
	sess, err := h.store.GetSession(ctx, orgID, sessionID)
	if err != nil {
		return nil, deperr.WrapDBIfTransient(fmt.Errorf("finalize: get session: %w", err))
	}

	// Build the per-commit list (PlanCommit DTOs).
	planCommits := make([]openapi.PlanCommit, 0, len(commits))
	for _, c := range commits {
		subject := firstLine(c.Message)
		accountID := lookupAccountID(ctx, h.store, c.Author.Email)
		pc := openapi.PlanCommit{
			Sha:         c.Hash.String(),
			AuthorName:  c.Author.Name,
			AuthorEmail: c.Author.Email,
			Subject:     subject,
			CommittedAt: c.Committer.When.UTC(),
		}
		if accountID != "" {
			pc.AccountId = accountID
		}
		planCommits = append(planCommits, pc)
	}

	mode := lock.Mode
	if mode == "" {
		mode = "squash"
	}

	// Compose the squash message (squash mode only).
	var squashMessageBody string
	var commitMessageField string
	var coAuthorsField []openapi.CoAuthor
	if mode == "squash" {
		override := ""
		if lock.CommitMessage != nil {
			override = *lock.CommitMessage
		}
		subject, body, cas := ComposeSquashMessage(sess.Goal, override, commits)
		// Fill in best-effort account_id for each co-author.
		for i := range cas {
			if id := lookupAccountID(ctx, h.store, cas[i].Email); id != "" {
				cas[i].AccountID = id
			}
		}
		squashMessageBody = RenderSquashMessageBody(subject, body, cas)
		commitMessageField = squashMessageBody

		coAuthorsField = make([]openapi.CoAuthor, len(cas))
		for i, ca := range cas {
			out := openapi.CoAuthor{
				Name:  ca.Name,
				Email: ca.Email,
			}
			if ca.AccountID != "" {
				out.AccountId = ca.AccountID
			}
			coAuthorsField[i] = out
		}
	}

	// Build the script body.
	script := BuildScript(ScriptInput{
		Mode:              mode,
		TargetBranch:      lock.TargetBranch,
		BaseSHA:           lock.BaseSHA,
		SelectedSHAs:      shas,
		SquashMessageBody: squashMessageBody,
	})

	// Fetch source — kind=https + portal smart-HTTP URL. Token is null;
	// the plugin mints one only on the dedicated fetch-token endpoint.
	remoteURL := fmt.Sprintf("%s/git/%s/%s.git", h.portalURL, orgID, sessionID)
	fetchSource := openapi.FetchSource{
		Kind:      openapi.Https,
		RemoteUrl: remoteURL,
	}

	resp := openapi.PlanResponse{
		PlanId:          sessionID + ":" + lock.ID,
		Mode:            openapi.PlanMode(mode),
		Script:          script,
		LockStatus:      lockStatus(lock, lock.AcquiredByAccountID == acc.ID),
		FetchSource:     fetchSource,
		SelectedCommits: planCommits,
		TargetBranch:    lock.TargetBranch,
		BaseSha:         lock.BaseSHA,
	}
	if mode == "squash" {
		resp.CommitMessage = commitMessageField
		resp.CoAuthors = coAuthorsField
	}

	return openapi.GetFinalizePlan200JSONResponse(resp), nil
}

// lookupAccountID returns the portal account ID for the given email, or
// empty string when no match is found. Errors that are not "not-found"
// propagate via a log line and the function returns empty (best-effort;
// the trailer still works on GitHub etc.).
func lookupAccountID(ctx context.Context, s store.Store, email string) string {
	if email == "" {
		return ""
	}
	acc, err := s.GetAccountByEmail(ctx, email)
	if err != nil {
		// Not-found is normal; other errors are swallowed best-effort.
		return ""
	}
	return acc.ID
}
