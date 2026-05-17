package finalizecmd

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"jamsesh/cmd/jamsesh/portalclient"
	"jamsesh/internal/api/openapi"
)

// planID is the parsed form of the opaque "<session>:<lock>" string
// the portal hands to the user via copy-to-clipboard in the curation
// view.
type planID struct {
	SessionID string
	LockID    string
}

// parsePlanID splits an opaque plan id on the first ":" and returns
// both halves. Both halves must be non-empty; anything else is a
// usage error.
func parsePlanID(s string) (planID, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return planID{}, errors.New("plan-id is empty; expected <session>:<lock>")
	}
	parts := strings.SplitN(s, ":", 2)
	if len(parts) != 2 {
		return planID{}, fmt.Errorf("plan-id %q is malformed; expected <session>:<lock>", s)
	}
	sess := strings.TrimSpace(parts[0])
	lock := strings.TrimSpace(parts[1])
	if sess == "" {
		return planID{}, fmt.Errorf("plan-id %q has empty session segment", s)
	}
	if lock == "" {
		return planID{}, fmt.Errorf("plan-id %q has empty lock segment", s)
	}
	return planID{SessionID: sess, LockID: lock}, nil
}

// Plan is the local-typed view of the portal's PlanResponse. It mirrors
// openapi.PlanResponse field-for-field; we keep our own struct so the
// rest of the package doesn't carry an openapi dependency in its API
// surface (the openapi types use openapi-types' Email/UUID wrappers
// elsewhere; here every field is a primitive so the mirror is cheap).
type Plan struct {
	PlanID          string
	Mode            string // "squash" | "preserve"
	TargetBranch    string
	BaseSHA         string
	Script          string
	CommitMessage   string     // squash only
	CoAuthors       []CoAuthor // squash only
	SelectedCommits []PlanCommit
	FetchSource     FetchSource
	LockStatus      LockStatus
}

// CoAuthor is the local mirror of openapi.CoAuthor.
type CoAuthor struct {
	Name      string
	Email     string
	AccountID string
}

// PlanCommit is the local mirror of openapi.PlanCommit.
type PlanCommit struct {
	SHA         string
	Subject     string
	AuthorName  string
	AuthorEmail string
	AccountID   string
	CommittedAt time.Time
}

// FetchSource is the local mirror of openapi.FetchSource.
type FetchSource struct {
	Kind           string // "local" | "https"
	Path           string
	RemoteURL      string
	TokenExpiresAt time.Time
}

// LockStatus is the local mirror of openapi.LockStatus.
type LockStatus struct {
	HeldByAccountID string
	IsCaller        bool
	LockID          string
	AcquiredAt      time.Time
	ExpiresAt       time.Time
	LastActivityAt  time.Time
}

// fetchPlan calls GET /api/orgs/<org>/sessions/<sid>/finalize-plan?lock_id=<lid>
// and converts the response into the local Plan shape.
//
// When lockID is empty (the `finalize --local` path), the lock_id query
// param is omitted — the portal returns 409 in that case, surfaced to
// the user as "open the portal first to start a finalize session".
func fetchPlan(ctx context.Context, pc *portalclient.Client, orgID, sessionID, lockID string) (*Plan, error) {
	path := fmt.Sprintf("/api/orgs/%s/sessions/%s/finalize-plan", orgID, sessionID)
	if lockID != "" {
		path += "?lock_id=" + lockID
	}
	raw, err := portalclient.GetJSON[openapi.PlanResponse](ctx, pc, path)
	if err != nil {
		return nil, err
	}
	return planFromOpenAPI(raw), nil
}

// planFromOpenAPI converts the generated openapi.PlanResponse into the
// local Plan type. Exported-test-helper-shaped because it's exercised
// directly by unit tests too.
func planFromOpenAPI(r openapi.PlanResponse) *Plan {
	p := &Plan{
		PlanID:        r.PlanId,
		Mode:          string(r.Mode),
		TargetBranch:  r.TargetBranch,
		BaseSHA:       r.BaseSha,
		Script:        r.Script,
		CommitMessage: r.CommitMessage,
		FetchSource: FetchSource{
			Kind:           string(r.FetchSource.Kind),
			Path:           r.FetchSource.Path,
			RemoteURL:      r.FetchSource.RemoteUrl,
			TokenExpiresAt: r.FetchSource.TokenExpiresAt,
		},
		LockStatus: LockStatus{
			HeldByAccountID: r.LockStatus.HeldByAccountId,
			IsCaller:        r.LockStatus.IsCaller,
			LockID:          r.LockStatus.LockId,
			AcquiredAt:      r.LockStatus.AcquiredAt,
			ExpiresAt:       r.LockStatus.ExpiresAt,
			LastActivityAt:  r.LockStatus.LastActivityAt,
		},
	}
	for _, c := range r.CoAuthors {
		p.CoAuthors = append(p.CoAuthors, CoAuthor{
			Name:      c.Name,
			Email:     c.Email,
			AccountID: c.AccountId,
		})
	}
	for _, c := range r.SelectedCommits {
		p.SelectedCommits = append(p.SelectedCommits, PlanCommit{
			SHA:         c.Sha,
			Subject:     c.Subject,
			AuthorName:  c.AuthorName,
			AuthorEmail: c.AuthorEmail,
			AccountID:   c.AccountId,
			CommittedAt: c.CommittedAt,
		})
	}
	return p
}

// selectedSHAs returns the cherry-pick-ordered SHA list. Convenience
// shim so execute/printScript don't both reach into SelectedCommits.
func (p *Plan) selectedSHAs() []string {
	out := make([]string, len(p.SelectedCommits))
	for i, c := range p.SelectedCommits {
		out[i] = c.SHA
	}
	return out
}
