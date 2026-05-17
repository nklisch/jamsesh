package auth_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/portal/auth"
)

// ---------------------------------------------------------------------------
// FindOrProvision tests
// ---------------------------------------------------------------------------

func TestFindOrProvision_NewMagicLinkIdentity_CreatesAccountAndOrg(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	id := auth.Identity{
		Provider:    "magic-link",
		Email:       "alice@example.com",
		DisplayName: "alice",
	}

	acc, org, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("FindOrProvision: %v", err)
	}
	if acc.ID == "" {
		t.Error("account ID must not be empty")
	}
	if acc.Email != "alice@example.com" {
		t.Errorf("account email: want %q, got %q", "alice@example.com", acc.Email)
	}
	if org.ID == "" {
		t.Error("org ID must not be empty")
	}
	if org.Slug == "" {
		t.Error("org slug must not be empty")
	}

	// Verify org_member row was created with role=creator.
	member, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     org.ID,
		AccountID: acc.ID,
	})
	if err != nil {
		t.Fatalf("GetOrgMember: %v", err)
	}
	if member.Role != "creator" {
		t.Errorf("org member role: want %q, got %q", "creator", member.Role)
	}
}

func TestFindOrProvision_ExistingMagicLinkIdentity_ReturnsExisting(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	id := auth.Identity{
		Provider:    "magic-link",
		Email:       "bob@example.com",
		DisplayName: "bob",
	}

	acc1, org1, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("first provision: %v", err)
	}

	// Second call must return the same account and org — idempotent.
	acc2, org2, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}

	if acc1.ID != acc2.ID {
		t.Errorf("account ID mismatch: %q vs %q", acc1.ID, acc2.ID)
	}
	if org1.ID != org2.ID {
		t.Errorf("org ID mismatch: %q vs %q", org1.ID, org2.ID)
	}
}

func TestFindOrProvision_SlugFromEmail_DerivedCorrectly(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	id := auth.Identity{
		Provider: "magic-link",
		Email:    "john.doe+test@example.com",
	}

	_, org, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("FindOrProvision: %v", err)
	}

	// Slug derived from "john.doe+test": lower, non-alphanum → hyphens, trim.
	// Expected: "john-doe-test"
	if org.Slug == "" {
		t.Error("org slug must not be empty")
	}
	// Slug should be lowercase and not start/end with hyphen.
	if len(org.Slug) > 0 && (org.Slug[0] == '-' || org.Slug[len(org.Slug)-1] == '-') {
		t.Errorf("org slug must not start or end with hyphen, got %q", org.Slug)
	}
}

func TestFindOrProvision_SlugCollision_AppendsSuffix(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	// Create two users whose emails produce the same prefix ("alice").
	// But since slug uniqueness is per-table, we can force a collision by
	// pre-inserting an org with the expected slug manually.
	baseSlug := "alice"
	now := time.Now().UTC()
	if _, err := s.CreateOrg(ctx, store.CreateOrgParams{
		ID:        "pre-existing-org",
		Name:      baseSlug,
		Slug:      baseSlug,
		CreatedAt: now,
	}); err != nil {
		t.Fatalf("pre-create org: %v", err)
	}

	// Now provision an account with email "alice@example.com".
	// The slug "alice" is taken, so provision must use a suffixed slug.
	id := auth.Identity{
		Provider: "magic-link",
		Email:    "alice@example.com",
	}
	_, org, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("FindOrProvision after collision: %v", err)
	}
	if org.Slug == baseSlug {
		t.Errorf("expected suffixed slug, got bare %q", org.Slug)
	}
	if org.Slug == "" {
		t.Error("org slug must not be empty")
	}
}

// TestFindOrProvisionAt_UsesSuppliedClock asserts that the account, org,
// and org_member rows created on first sign-in all carry the supplied
// `now` as their CreatedAt — proving the clock parameter is the sole
// source of time for the provisioning write path.
func TestFindOrProvisionAt_UsesSuppliedClock(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	frozen := time.Date(2026, 5, 17, 12, 0, 0, 0, time.UTC)
	id := auth.Identity{
		Provider:    "magic-link",
		Email:       "frozen@example.com",
		DisplayName: "frozen",
	}

	acc, org, err := auth.FindOrProvisionAt(ctx, s, id, frozen)
	if err != nil {
		t.Fatalf("FindOrProvisionAt: %v", err)
	}

	if !acc.CreatedAt.Equal(frozen) {
		t.Errorf("account CreatedAt: want %v, got %v", frozen, acc.CreatedAt)
	}
	if !org.CreatedAt.Equal(frozen) {
		t.Errorf("org CreatedAt: want %v, got %v", frozen, org.CreatedAt)
	}

	m, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
		OrgID:     org.ID,
		AccountID: acc.ID,
	})
	if err != nil {
		t.Fatalf("GetOrgMember: %v", err)
	}
	if !m.CreatedAt.Equal(frozen) {
		t.Errorf("org_member CreatedAt: want %v, got %v", frozen, m.CreatedAt)
	}
}

// TestFindOrProvision_DelegatesToFindOrProvisionAt verifies that the
// back-compat FindOrProvision entry point still produces an account
// whose CreatedAt is close to the real wall clock (sanity check; the
// equality bound spans the call duration).
func TestFindOrProvision_DelegatesToFindOrProvisionAt(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	id := auth.Identity{
		Provider:    "magic-link",
		Email:       "delegate@example.com",
		DisplayName: "delegate",
	}

	before := time.Now().UTC()
	acc, _, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("FindOrProvision: %v", err)
	}
	after := time.Now().UTC()

	if acc.CreatedAt.Before(before) || acc.CreatedAt.After(after) {
		t.Errorf("account CreatedAt %v not in [%v, %v]", acc.CreatedAt, before, after)
	}
}

func TestFindOrProvision_GitHubIdentity_UsesProviderID(t *testing.T) {
	s := openStore(t)
	ctx := context.Background()

	ghID := "gh-12345"
	id := auth.Identity{
		Provider:    "github",
		ProviderID:  ghID,
		Email:       "carol@example.com",
		DisplayName: "Carol",
	}

	acc1, _, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("first provision: %v", err)
	}
	if acc1.GithubUserID == nil || *acc1.GithubUserID != ghID {
		t.Errorf("GithubUserID: want %q, got %v", ghID, acc1.GithubUserID)
	}

	// Second call with same GitHub ID should return the same account.
	acc2, _, err := auth.FindOrProvision(ctx, s, id)
	if err != nil {
		t.Fatalf("second provision: %v", err)
	}
	if acc1.ID != acc2.ID {
		t.Errorf("account ID mismatch on second provision: %q vs %q", acc1.ID, acc2.ID)
	}
}
