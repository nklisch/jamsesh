package store_test

// cross_org_test.go — org_id discipline test suite
//
// These tests form the structural guarantee that no session-table query can
// accidentally cross tenant boundaries. The suite runs against every dialect
// returned by stores(t).
//
// FAILURE MODE VERIFICATION
// -------------------------
// A reviewer wishing to confirm that these tests catch a real regression can
// temporarily remove the `org_id = ?` (SQLite) or `org_id = $1` (Postgres)
// predicate from, for example, the GetSession query in
// internal/db/sqlitestore/queries.sql.go and run:
//
//   go test ./internal/db/store/... -run TestOrgIDDiscipline
//
// TestOrgIDDiscipline/sqlite/GetSession_cross_org_returns_ErrNotFound will
// fail with:
//   expected ErrNotFound for cross-org GetSession, got <session row>
//
// Restoring the predicate makes the suite green again. This documents that
// the predicate is load-bearing and that these tests catch its removal.

import (
	"context"
	"errors"
	"testing"

	"jamsesh/internal/db/store"
)

// TestOrgIDDiscipline is the cross-org leakage suite. It verifies that every
// org-scoped session read and write silently (or loudly, via ErrNotFound)
// respects the org_id boundary.
func TestOrgIDDiscipline(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			// ── Fixture setup ──────────────────────────────────────────────
			//
			// orgA and orgB are independent tenants.
			// accA is a member of orgA only.
			// sessA belongs to orgA; sessB belongs to orgB.
			// accA is a session-member of sessA.

			orgA := mustCreateOrg(t, ctx, s, "org-a-"+tt.name)
			orgB := mustCreateOrg(t, ctx, s, "org-b-"+tt.name)
			accA := mustCreateAccount(t, ctx, s, "alice-"+tt.name+"@example.com")

			mustAddOrgMember(t, ctx, s, orgA.ID, accA.ID, "creator")
			sessA := mustCreateSession(t, ctx, s, orgA.ID, "sess-a-"+tt.name)
			sessB := mustCreateSession(t, ctx, s, orgB.ID, "sess-b-"+tt.name)
			mustAddSessionMember(t, ctx, s, orgA.ID, sessA.ID, accA.ID, "member")

			// ── 1. GetSession cross-org returns ErrNotFound ────────────────

			t.Run("GetSession_cross_org_returns_ErrNotFound", func(t *testing.T) {
				_, err := s.GetSession(ctx, orgA.ID, sessB.ID)
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound for cross-org GetSession, got %v", err)
				}
			})

			// ── 2. ListSessionsForOrg does not include the other org's session ──

			t.Run("ListSessionsForOrg_excludes_other_org", func(t *testing.T) {
				list, err := s.ListSessionsForOrg(ctx, orgA.ID)
				assertNoError(t, err)
				assertOnlyContains(t, list, sessA.ID)

				// Verify from orgB's perspective too.
				listB, err := s.ListSessionsForOrg(ctx, orgB.ID)
				assertNoError(t, err)
				assertOnlyContains(t, listB, sessB.ID)
			})

			// ── 3. UpdateSessionStatus with wrong org_id is a no-op ────────
			//
			// The implementation uses WHERE org_id = ? AND id = ?, so a
			// mismatched org silently matches 0 rows. The update must not
			// change the session in its owning org.

			t.Run("UpdateSessionStatus_cross_org_is_noop", func(t *testing.T) {
				// Attempt to update sessB using orgA's ID.
				err := s.UpdateSessionStatus(ctx, store.UpdateSessionStatusParams{
					OrgID:  orgA.ID,
					ID:     sessB.ID,
					Status: "ended",
				})
				assertNoError(t, err) // no error: 0 rows updated is fine

				// sessB should still be "active" when read from its real org.
				got, err := s.GetSession(ctx, orgB.ID, sessB.ID)
				assertNoError(t, err)
				if got.Status != "active" {
					t.Errorf("cross-org UpdateSessionStatus mutated sessB: status = %q, want %q",
						got.Status, "active")
				}
			})

			// ── 4. SetSessionBaseSHA with wrong org_id is a no-op ──────────

			t.Run("SetSessionBaseSHA_cross_org_is_noop", func(t *testing.T) {
				sha := "deadbeef"
				err := s.SetSessionBaseSHA(ctx, store.SetSessionBaseSHAParams{
					OrgID:   orgA.ID,
					ID:      sessB.ID,
					BaseSHA: &sha,
				})
				assertNoError(t, err)

				got, err := s.GetSession(ctx, orgB.ID, sessB.ID)
				assertNoError(t, err)
				if got.BaseSHA != nil {
					t.Errorf("cross-org SetSessionBaseSHA mutated sessB: base_sha = %q, want nil",
						*got.BaseSHA)
				}
			})

			// ── 5. GetSessionMember with wrong org_id returns ErrNotFound ──

			t.Run("GetSessionMember_cross_org_returns_ErrNotFound", func(t *testing.T) {
				// accA is a member of sessA/orgA. Reading via orgB must fail.
				_, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
					OrgID:     orgB.ID,
					SessionID: sessA.ID,
					AccountID: accA.ID,
				})
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound for cross-org GetSessionMember, got %v", err)
				}
			})

			// ── 6. ListSessionMembers with wrong org_id returns empty list ──

			t.Run("ListSessionMembers_cross_org_returns_empty", func(t *testing.T) {
				members, err := s.ListSessionMembers(ctx, store.ListSessionMembersParams{
					OrgID:     orgB.ID, // wrong org
					SessionID: sessA.ID,
				})
				assertNoError(t, err)
				if len(members) != 0 {
					t.Errorf("cross-org ListSessionMembers returned %d members, want 0", len(members))
				}
			})

			// ── 7. RemoveSessionMember with wrong org_id is a no-op ────────

			t.Run("RemoveSessionMember_cross_org_is_noop", func(t *testing.T) {
				// Attempt to remove accA from sessA using orgB's ID.
				err := s.RemoveSessionMember(ctx, store.RemoveSessionMemberParams{
					OrgID:     orgB.ID, // wrong org
					SessionID: sessA.ID,
					AccountID: accA.ID,
				})
				assertNoError(t, err)

				// accA should still be a member when queried from the real org.
				member, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
					OrgID:     orgA.ID,
					SessionID: sessA.ID,
					AccountID: accA.ID,
				})
				assertNoError(t, err)
				if member.AccountID != accA.ID {
					t.Errorf("cross-org RemoveSessionMember removed the member from real org")
				}
			})
		})
	}
}

// TestListSessionMembershipsForAccount_CrossOrgException documents and tests
// the one intentional cross-org read: an account can see all its session
// memberships across every org, with org_id populated on each row so the
// caller can issue subsequent scoped queries.
func TestListSessionMembershipsForAccount_CrossOrgException(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.open(t)

			// acc is a member of sessions in two distinct orgs.
			orgA := mustCreateOrg(t, ctx, s, "cross-a-"+tt.name)
			orgB := mustCreateOrg(t, ctx, s, "cross-b-"+tt.name)
			acc := mustCreateAccount(t, ctx, s, "multi-"+tt.name+"@example.com")

			mustAddOrgMember(t, ctx, s, orgA.ID, acc.ID, "creator")
			mustAddOrgMember(t, ctx, s, orgB.ID, acc.ID, "member")

			sessA := mustCreateSession(t, ctx, s, orgA.ID, "sess-in-a-"+tt.name)
			sessB := mustCreateSession(t, ctx, s, orgB.ID, "sess-in-b-"+tt.name)

			mustAddSessionMember(t, ctx, s, orgA.ID, sessA.ID, acc.ID, "member")
			mustAddSessionMember(t, ctx, s, orgB.ID, sessB.ID, acc.ID, "member")

			memberships, err := s.ListSessionMembershipsForAccount(ctx, acc.ID)
			assertNoError(t, err)

			if len(memberships) != 2 {
				t.Fatalf("expected 2 memberships across orgs, got %d", len(memberships))
			}

			// Build a map by session ID for easy assertion.
			bySession := make(map[string]store.SessionMembership, len(memberships))
			for _, m := range memberships {
				bySession[m.SessionID] = m
			}

			mA, ok := bySession[sessA.ID]
			if !ok {
				t.Fatalf("membership for sessA (%q) missing from result", sessA.ID)
			}
			if mA.OrgID != orgA.ID {
				t.Errorf("sessA membership: org_id = %q, want %q", mA.OrgID, orgA.ID)
			}

			mB, ok := bySession[sessB.ID]
			if !ok {
				t.Fatalf("membership for sessB (%q) missing from result", sessB.ID)
			}
			if mB.OrgID != orgB.ID {
				t.Errorf("sessB membership: org_id = %q, want %q", mB.OrgID, orgB.ID)
			}

			// Verify that all membership rows carry the account ID back.
			for _, m := range memberships {
				if m.AccountID != acc.ID {
					t.Errorf("membership account_id = %q, want %q", m.AccountID, acc.ID)
				}
			}
		})
	}
}
