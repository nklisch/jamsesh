package finalize

import (
	"context"
	"errors"

	"jamsesh/internal/db/store"
)

// membershipVerdict is the result of a member-of-session check used by the
// lock endpoints. Each handler maps the verdict to its endpoint-specific
// 403 / 404 response object — Go's generics are not expressive enough to
// share a single helper that returns the strict-server response interface.
type membershipVerdict int

const (
	memberOK membershipVerdict = iota
	memberNotOrgMember
	memberNotSessionMember
	memberSessionNotFound
)

// checkSessionMembership verifies the caller is a member of both the org
// and the session. Returns a verdict and any unexpected error. Session-not-
// found returns memberSessionNotFound (handlers translate to 404).
func checkSessionMembership(ctx context.Context, s store.Store, orgID, sessionID, accountID string) (membershipVerdict, error) {
	if _, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     orgID,
		AccountID: accountID,
	}); err != nil {
		if errors.Is(err, store.ErrNotFound) {
			return memberNotOrgMember, nil
		}
		return memberOK, err
	}
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
