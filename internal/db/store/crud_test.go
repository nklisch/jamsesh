package store_test

// crud_test.go — Create/Get/List/Update/Delete round-trips for every table.
//
// These tests run against all dialects via the stores(t) harness. They focus
// on verifying that data survives the adapter translation layer intact.

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db/store"
)

// TestOrgCRUD verifies Create + GetByID + GetBySlug.
func TestOrgCRUD(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			now := time.Now().UTC().Truncate(time.Second)
			org, err := s.CreateOrg(ctx, store.CreateOrgParams{
				ID:        nextID("org-crud"),
				Name:      "CRUD Org",
				Slug:      "crud-org-" + tt.name,
				CreatedAt: now,
			})
			assertNoError(t, err)

			byID, err := s.GetOrgByID(ctx, org.ID)
			assertNoError(t, err)
			if byID.Name != "CRUD Org" {
				t.Errorf("GetOrgByID name: got %q, want %q", byID.Name, "CRUD Org")
			}

			bySlug, err := s.GetOrgBySlug(ctx, org.Slug)
			assertNoError(t, err)
			if bySlug.ID != org.ID {
				t.Errorf("GetOrgBySlug id: got %q, want %q", bySlug.ID, org.ID)
			}
		})
	}
}

// TestAccountCRUD verifies Create + GetByID + GetByEmail + GetByGitHubUserID
// + UpdateDisplayName.
func TestAccountCRUD(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			now := time.Now().UTC().Truncate(time.Second)
			ghID := "gh-99"
			acc, err := s.CreateAccount(ctx, store.CreateAccountParams{
				ID:           nextID("acc-crud"),
				Email:        "crud-" + tt.name + "@example.com",
				DisplayName:  "CRUD User",
				GithubUserID: &ghID,
				CreatedAt:    now,
			})
			assertNoError(t, err)

			byID, err := s.GetAccountByID(ctx, acc.ID)
			assertNoError(t, err)
			if byID.Email != acc.Email {
				t.Errorf("GetAccountByID email: got %q, want %q", byID.Email, acc.Email)
			}
			if byID.GithubUserID == nil || *byID.GithubUserID != ghID {
				t.Errorf("GetAccountByID github_user_id: got %v, want %q", byID.GithubUserID, ghID)
			}

			byEmail, err := s.GetAccountByEmail(ctx, acc.Email)
			assertNoError(t, err)
			if byEmail.ID != acc.ID {
				t.Errorf("GetAccountByEmail id: got %q, want %q", byEmail.ID, acc.ID)
			}

			byGH, err := s.GetAccountByGitHubUserID(ctx, &ghID)
			assertNoError(t, err)
			if byGH.ID != acc.ID {
				t.Errorf("GetAccountByGitHubUserID id: got %q, want %q", byGH.ID, acc.ID)
			}

			// UpdateDisplayName
			err = s.UpdateAccountDisplayName(ctx, store.UpdateAccountDisplayNameParams{
				ID:          acc.ID,
				DisplayName: "Updated Name",
			})
			assertNoError(t, err)

			updated, err := s.GetAccountByID(ctx, acc.ID)
			assertNoError(t, err)
			if updated.DisplayName != "Updated Name" {
				t.Errorf("UpdateDisplayName: got %q, want %q", updated.DisplayName, "Updated Name")
			}
		})
	}
}

// TestOrgMemberCRUD verifies AddOrgMember + GetOrgMember + ListOrgMembers
// + ListOrgsForAccount + RemoveOrgMember.
func TestOrgMemberCRUD(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			org := mustCreateOrg(t, ctx, s, "org-member-"+tt.name)
			acc := mustCreateAccount(t, ctx, s, "member-"+tt.name+"@example.com")

			mustAddOrgMember(t, ctx, s, org.ID, acc.ID, "creator")

			// GetOrgMember
			member, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
				OrgID:     org.ID,
				AccountID: acc.ID,
			})
			assertNoError(t, err)
			if member.Role != "creator" {
				t.Errorf("GetOrgMember role: got %q, want %q", member.Role, "creator")
			}

			// ListOrgMembers
			members, err := s.ListOrgMembers(ctx, org.ID)
			assertNoError(t, err)
			if len(members) != 1 {
				t.Fatalf("ListOrgMembers: got %d, want 1", len(members))
			}
			if members[0].AccountID != acc.ID {
				t.Errorf("ListOrgMembers[0].AccountID = %q, want %q", members[0].AccountID, acc.ID)
			}

			// ListOrgsForAccount
			orgs, err := s.ListOrgsForAccount(ctx, acc.ID)
			assertNoError(t, err)
			if len(orgs) != 1 || orgs[0].ID != org.ID {
				t.Errorf("ListOrgsForAccount: got %v, want [%q]", orgs, org.ID)
			}

			// RemoveOrgMember
			err = s.RemoveOrgMember(ctx, store.RemoveOrgMemberParams{
				OrgID:     org.ID,
				AccountID: acc.ID,
			})
			assertNoError(t, err)

			_, err = s.GetOrgMember(ctx, store.GetOrgMemberParams{
				OrgID:     org.ID,
				AccountID: acc.ID,
			})
			if err == nil {
				t.Error("GetOrgMember after RemoveOrgMember: expected error, got nil")
			}
		})
	}
}

// TestSessionCRUDDialects is the parameterized companion to TestSessionCRUD in
// store_test.go (which is SQLite-only). This suite covers UpdateSessionStatus
// and SetSessionBaseSHA as well.
func TestSessionCRUDDialects(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			org := mustCreateOrg(t, ctx, s, "sess-crud-"+tt.name)
			sess := mustCreateSession(t, ctx, s, org.ID, "sprint-"+tt.name)

			// GetSession
			got, err := s.GetSession(ctx, org.ID, sess.ID)
			assertNoError(t, err)
			if got.Status != "active" {
				t.Errorf("initial status: got %q, want %q", got.Status, "active")
			}
			if got.BaseSHA != nil {
				t.Errorf("initial base_sha: got %v, want nil", got.BaseSHA)
			}

			// UpdateSessionStatus
			err = s.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
				OrgID:  org.ID,
				ID:     sess.ID,
				Status: "ended",
			})
			assertNoError(t, err)

			after, err := s.GetSession(ctx, org.ID, sess.ID)
			assertNoError(t, err)
			if after.Status != "ended" {
				t.Errorf("after UpdateSessionStatus: got %q, want %q", after.Status, "ended")
			}

			// SetSessionBaseSHA
			sha := "abc123"
			err = s.SetSessionBaseSHA(ctx, store.SetSessionBaseSHAParams{
				OrgID:   org.ID,
				ID:      sess.ID,
				BaseSHA: &sha,
			})
			assertNoError(t, err)

			withSHA, err := s.GetSession(ctx, org.ID, sess.ID)
			assertNoError(t, err)
			if withSHA.BaseSHA == nil || *withSHA.BaseSHA != sha {
				t.Errorf("after SetSessionBaseSHA: got %v, want %q", withSHA.BaseSHA, sha)
			}

			// ListSessionsForOrg
			list, err := s.ListSessionsForOrg(ctx, org.ID)
			assertNoError(t, err)
			assertOnlyContains(t, list, sess.ID)
		})
	}
}

// TestSessionMemberCRUD verifies AddSessionMember + GetSessionMember +
// ListSessionMembers + RemoveSessionMember.
func TestSessionMemberCRUD(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			org := mustCreateOrg(t, ctx, s, "sm-crud-"+tt.name)
			acc := mustCreateAccount(t, ctx, s, "sm-"+tt.name+"@example.com")
			sess := mustCreateSession(t, ctx, s, org.ID, "sm-sess-"+tt.name)

			mustAddSessionMember(t, ctx, s, org.ID, sess.ID, acc.ID, "member")

			// GetSessionMember
			member, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
				OrgID:     org.ID,
				SessionID: sess.ID,
				AccountID: acc.ID,
			})
			assertNoError(t, err)
			if member.Role != "member" {
				t.Errorf("GetSessionMember role: got %q, want %q", member.Role, "member")
			}

			// ListSessionMembers
			members, err := s.ListSessionMembers(ctx, store.ListSessionMembersParams{
				OrgID:     org.ID,
				SessionID: sess.ID,
			})
			assertNoError(t, err)
			assertSessionMemberIDs(t, members, acc.ID)

			// RemoveSessionMember
			err = s.RemoveSessionMember(ctx, store.RemoveSessionMemberParams{
				OrgID:     org.ID,
				SessionID: sess.ID,
				AccountID: acc.ID,
			})
			assertNoError(t, err)

			members, err = s.ListSessionMembers(ctx, store.ListSessionMembersParams{
				OrgID:     org.ID,
				SessionID: sess.ID,
			})
			assertNoError(t, err)
			if len(members) != 0 {
				t.Errorf("after RemoveSessionMember: got %d members, want 0", len(members))
			}
		})
	}
}

// TestOAuthTokenCRUD verifies Create + GetByHash + Touch + Revoke + List +
// RevokeAllForAccount.
func TestOAuthTokenCRUD(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			acc := mustCreateAccount(t, ctx, s, "oauth-"+tt.name+"@example.com")
			now := time.Now().UTC().Truncate(time.Second)

			tok, err := s.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
				ID:         nextID("tok"),
				AccountID:  acc.ID,
				TokenHash:  "hash-" + tt.name,
				Kind:       "access",
				IssuedAt:   now,
				ExpiresAt:  now.Add(time.Hour),
				LastUsedAt: nil,
				RevokedAt:  nil,
			})
			assertNoError(t, err)

			// GetByHash
			byHash, err := s.GetOAuthTokenByHash(ctx, "hash-"+tt.name)
			assertNoError(t, err)
			if byHash.ID != tok.ID {
				t.Errorf("GetOAuthTokenByHash id: got %q, want %q", byHash.ID, tok.ID)
			}

			// TouchLastUsed
			lastUsed := now.Add(time.Minute)
			err = s.TouchOAuthTokenLastUsed(ctx, store.TouchOAuthTokenLastUsedParams{
				ID:         tok.ID,
				LastUsedAt: &lastUsed,
			})
			assertNoError(t, err)

			// ListOAuthTokensForAccount
			list, err := s.ListOAuthTokensForAccount(ctx, acc.ID)
			assertNoError(t, err)
			if len(list) != 1 {
				t.Fatalf("ListOAuthTokensForAccount: got %d, want 1", len(list))
			}
			if list[0].LastUsedAt == nil {
				t.Error("LastUsedAt should be set after Touch")
			}

			// Revoke single
			revokedAt := now.Add(2 * time.Minute)
			err = s.RevokeOAuthToken(ctx, store.RevokeOAuthTokenParams{
				ID:        tok.ID,
				RevokedAt: &revokedAt,
			})
			assertNoError(t, err)

			after, err := s.GetOAuthTokenByHash(ctx, "hash-"+tt.name)
			assertNoError(t, err)
			if after.RevokedAt == nil {
				t.Error("RevokedAt should be set after RevokeOAuthToken")
			}

			// RevokeAll — create a second token then bulk-revoke.
			_, err = s.CreateOAuthToken(ctx, store.CreateOAuthTokenParams{
				ID:        nextID("tok2"),
				AccountID: acc.ID,
				TokenHash: "hash2-" + tt.name,
				Kind:      "refresh",
				IssuedAt:  now,
				ExpiresAt: now.Add(24 * time.Hour),
			})
			assertNoError(t, err)

			err = s.RevokeAllOAuthTokensForAccount(ctx, store.RevokeAllOAuthTokensForAccountParams{
				AccountID: acc.ID,
				RevokedAt: &revokedAt,
			})
			assertNoError(t, err)
		})
	}
}

// TestMagicLinkTokenCRUD verifies Create + GetByHash + ConsumeMagicLinkToken.
// (Single-use enforcement via used_at IS NULL is tested more deeply in
// store_test.go:TestMagicLinkSingleUse.)
func TestMagicLinkTokenCRUD(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			now := time.Now().UTC().Truncate(time.Second)
			tok, err := s.CreateMagicLinkToken(ctx, store.CreateMagicLinkTokenParams{
				ID:        nextID("ml"),
				TokenHash: "ml-hash-" + tt.name,
				Email:     "magic-" + tt.name + "@example.com",
				IssuedAt:  now,
				ExpiresAt: now.Add(15 * time.Minute),
			})
			assertNoError(t, err)

			// GetByHash
			got, err := s.GetMagicLinkTokenByHash(ctx, "ml-hash-"+tt.name)
			assertNoError(t, err)
			if got.ID != tok.ID {
				t.Errorf("GetMagicLinkTokenByHash id: got %q, want %q", got.ID, tok.ID)
			}
			if got.UsedAt != nil {
				t.Errorf("initial UsedAt: got %v, want nil", got.UsedAt)
			}

			// Consume
			usedAt := now.Add(time.Minute)
			err = s.ConsumeMagicLinkToken(ctx, store.ConsumeMagicLinkTokenParams{
				ID:     tok.ID,
				UsedAt: &usedAt,
			})
			assertNoError(t, err)

			after, err := s.GetMagicLinkTokenByHash(ctx, "ml-hash-"+tt.name)
			assertNoError(t, err)
			if after.UsedAt == nil {
				t.Error("UsedAt should be set after ConsumeMagicLinkToken")
			}
		})
	}
}
