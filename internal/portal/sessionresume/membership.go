package sessionresume

import (
	"context"
	"errors"

	"jamsesh/internal/db/store"
)

// membershipVerdict is the result of a member-of-session check used by the
// mint endpoint. Each handler maps the verdict to its endpoint-specific
// 403 / 404 response object — Go's generics are not expressive enough to
// share a single helper that returns the strict-server response interface.
type membershipVerdict int

const (
	memberOK membershipVerdict = iota
	memberNotOrgMember
	memberNotSessionMember
	memberSessionNotFound
)

// checkSessionMembership verifies the caller is a member of the session,
// and for non-playground orgs also verifies org membership. Returns a verdict
// and any unexpected error. Session-not-found returns memberSessionNotFound
// (handlers translate to 404).
//
// Playground (anonymous) accounts are session members only — they are never
// added to the playground org's org_members table, so gating on org membership
// would produce a spurious 403 for every real anonymous bearer. This mirrors
// handlerauth.RequireAnonymousSessionMember (session-only) vs
// handlerauth.RequireOrgMember (org path).
//
// Replicates finalize.checkSessionMembership (which is package-private to
// finalize). The two copies are intentional — the helper is package-private
// and cannot be shared without creating an import or re-exporting it.
func checkSessionMembership(ctx context.Context, s sessionResumeStore, orgID, sessionID, accountID string) (membershipVerdict, error) {
	if orgID != playgroundOrgID {
		// Durable orgs: verify org membership first.
		if _, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
			OrgID:     orgID,
			AccountID: accountID,
		}); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return memberNotOrgMember, nil
			}
			return memberOK, err
		}
	}
	// Both durable and playground: verify session membership.
	if _, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
		OrgID:     orgID,
		SessionID: sessionID,
		AccountID: accountID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			// Either the session doesn't exist or the caller isn't a
			// member — distinguish for 404 vs 403.
			if _, sErr := s.GetSession(ctx, orgID, sessionID); sErr != nil {
				if errors.Is(sErr, store.ErrNotFound) {
					return memberSessionNotFound, nil
				}
				return memberOK, sErr
			}
			return memberNotSessionMember, nil
		}
		return memberOK, err
	}
	return memberOK, nil
}
