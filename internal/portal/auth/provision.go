// Package auth implements authentication flow handlers and helpers.
package auth

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/google/uuid"

	"jamsesh/internal/db/store"
)

// Identity describes a user's identity as provided by an auth flow.
type Identity struct {
	// Provider is "magic-link" or "github".
	Provider string
	// ProviderID is the GitHub user ID (string form) for OAuth flows.
	// Empty for magic-link.
	ProviderID string
	// Email is the verified email address.
	Email string
	// DisplayName is a human-readable name derived from the provider.
	// For magic-link this is typically the email prefix.
	DisplayName string
}

// FindOrProvision returns the account+org pair for the given identity,
// creating them on first sign-in. Idempotent: calling twice with the same
// identity returns the same account and org. Uses the real system clock
// for provisioning timestamps; clock-injectable callers use
// FindOrProvisionAt.
//
// Algorithm:
//  1. Look up account by GitHub user ID (OAuth) or email (magic-link).
//  2. If found: return account + its primary org (first ListOrgsForAccount row).
//  3. If not found: create account (UUID id), create org (slug from email
//     prefix, suffix with 6 random alphanum chars on slug collision), insert
//     org_member with role "creator", return both.
func FindOrProvision(ctx context.Context, s store.Store, id Identity) (store.Account, store.Org, error) {
	return FindOrProvisionAt(ctx, s, id, time.Now().UTC())
}

// FindOrProvisionAt is the clock-injectable variant of FindOrProvision.
// Callers (e.g. MagicLinkHandler with an injectable Clock) pass their
// clock's Now() so test-clock advancement is observable in the resulting
// CreatedAt fields. See FindOrProvision for the algorithm.
func FindOrProvisionAt(ctx context.Context, s store.Store, id Identity, now time.Time) (store.Account, store.Org, error) {
	// Step 1: look up existing account.
	acc, err := lookupAccount(ctx, s, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return store.Account{}, store.Org{}, fmt.Errorf("auth: lookup account: %w", err)
	}

	if err == nil {
		// Step 2: account exists; return it with its primary org.
		org, orgErr := primaryOrg(ctx, s, acc.ID)
		if orgErr != nil {
			return store.Account{}, store.Org{}, fmt.Errorf("auth: primary org for %s: %w", acc.ID, orgErr)
		}
		return acc, org, nil
	}

	// Step 3: provision new account + org.
	return createAccountAndOrg(ctx, s, id, now)
}

// lookupAccount attempts to find an existing account by the most specific
// identifier available for the provider.
func lookupAccount(ctx context.Context, s store.Store, id Identity) (store.Account, error) {
	if id.Provider == "github" && id.ProviderID != "" {
		return s.GetAccountByGitHubUserID(ctx, &id.ProviderID)
	}
	return s.GetAccountByEmail(ctx, id.Email)
}

// primaryOrg returns the first org the account belongs to. Every provisioned
// account has exactly one org (their personal org created at sign-up).
func primaryOrg(ctx context.Context, s store.Store, accountID string) (store.Org, error) {
	orgs, err := s.ListOrgsForAccount(ctx, accountID)
	if err != nil {
		return store.Org{}, fmt.Errorf("list orgs: %w", err)
	}
	if len(orgs) == 0 {
		return store.Org{}, fmt.Errorf("account %s has no org membership", accountID)
	}
	return orgs[0], nil
}

// createAccountAndOrg creates a new account, a new personal org, and links
// them with an org_member row (role=creator). Returns both. The now
// parameter stamps CreatedAt on every inserted row so a single instant
// is shared across account, org, and org_member.
func createAccountAndOrg(ctx context.Context, s store.Store, id Identity, now time.Time) (store.Account, store.Org, error) {
	accountID := uuid.New().String()

	displayName := id.DisplayName
	if displayName == "" {
		displayName = emailPrefix(id.Email)
	}

	var githubUserID *string
	if id.Provider == "github" && id.ProviderID != "" {
		githubUserID = &id.ProviderID
	}

	acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
		ID:           accountID,
		Email:        id.Email,
		DisplayName:  displayName,
		GithubUserID: githubUserID,
		CreatedAt:    now,
	})
	if err != nil {
		return store.Account{}, store.Org{}, fmt.Errorf("auth: create account: %w", err)
	}

	org, err := CreateOrgWithSlug(ctx, s, emailPrefix(id.Email), now)
	if err != nil {
		return store.Account{}, store.Org{}, err
	}

	if err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
		OrgID:     org.ID,
		AccountID: accountID,
		Role:      "creator",
		CreatedAt: now,
	}); err != nil {
		return store.Account{}, store.Org{}, fmt.Errorf("auth: add org member: %w", err)
	}

	return acc, org, nil
}

// emailPrefix returns the part of an email address before "@".
func emailPrefix(email string) string {
	if i := strings.Index(email, "@"); i > 0 {
		return email[:i]
	}
	return email
}

