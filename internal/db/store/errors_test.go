package store_test

// errors_test.go — error normalization across both dialects.
//
// Every Get* and Create* path that can produce ErrNotFound or
// ErrUniqueViolation is exercised here against every dialect returned by
// stores(t). This guarantees that dialect-specific error types are always
// translated to the canonical store sentinels before they reach callers.

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db/store"
)

// TestErrNotFoundAllDialects covers the ErrNotFound paths for every entity
// type (org, account, session, session member, oauth token, magic-link token).
func TestErrNotFoundAllDialects(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			t.Run("GetOrgByID", func(t *testing.T) {
				_, err := s.GetOrgByID(ctx, "nonexistent-org")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetOrgBySlug", func(t *testing.T) {
				_, err := s.GetOrgBySlug(ctx, "no-such-slug")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetAccountByID", func(t *testing.T) {
				_, err := s.GetAccountByID(ctx, "nonexistent-acc")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetAccountByEmail", func(t *testing.T) {
				_, err := s.GetAccountByEmail(ctx, "nobody@example.com")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetAccountByGitHubUserID", func(t *testing.T) {
				ghID := "gh-nonexistent"
				_, err := s.GetAccountByGitHubUserID(ctx, &ghID)
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetOrgMember", func(t *testing.T) {
				_, err := s.GetOrgMember(ctx, store.GetOrgMemberParams{
					OrgID:     "no-org",
					AccountID: "no-acc",
				})
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetSession", func(t *testing.T) {
				_, err := s.GetSession(ctx, "no-org", "no-sess")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetSessionMember", func(t *testing.T) {
				_, err := s.GetSessionMember(ctx, store.GetSessionMemberParams{
					OrgID:     "no-org",
					SessionID: "no-sess",
					AccountID: "no-acc",
				})
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetOAuthTokenByHash", func(t *testing.T) {
				_, err := s.GetOAuthTokenByHash(ctx, "no-hash")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})

			t.Run("GetMagicLinkTokenByHash", func(t *testing.T) {
				_, err := s.GetMagicLinkTokenByHash(ctx, "no-hash")
				if !errors.Is(err, store.ErrNotFound) {
					t.Fatalf("expected ErrNotFound, got %v", err)
				}
			})
		})
	}
}

// TestErrUniqueViolationAllDialects covers UNIQUE constraint violations for
// every entity type that has a uniqueness constraint.
func TestErrUniqueViolationAllDialects(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC()

			t.Run("CreateOrg_duplicate_slug", func(t *testing.T) {
				slug := "dup-slug-" + tt.Name
				p := store.CreateOrgParams{
					ID:        nextID("org-dup1"),
					Name:      "Org Dup 1",
					Slug:      slug,
					CreatedAt: now,
				}
				if _, err := s.CreateOrg(ctx, p); err != nil {
					t.Fatalf("first CreateOrg: %v", err)
				}

				p.ID = nextID("org-dup2")
				p.Name = "Org Dup 2"
				_, err := s.CreateOrg(ctx, p)
				if !errors.Is(err, store.ErrUniqueViolation) {
					t.Fatalf("expected ErrUniqueViolation on duplicate slug, got %v", err)
				}
			})

			t.Run("CreateAccount_duplicate_email", func(t *testing.T) {
				email := "dup-" + tt.Name + "@example.com"
				p := store.CreateAccountParams{
					ID:        nextID("acc-dup1"),
					Email:     email,
					DisplayName: "Dup 1",
					CreatedAt: now,
				}
				if _, err := s.CreateAccount(ctx, p); err != nil {
					t.Fatalf("first CreateAccount: %v", err)
				}

				p.ID = nextID("acc-dup2")
				p.DisplayName = "Dup 2"
				_, err := s.CreateAccount(ctx, p)
				if !errors.Is(err, store.ErrUniqueViolation) {
					t.Fatalf("expected ErrUniqueViolation on duplicate email, got %v", err)
				}
			})

			t.Run("CreateMagicLinkToken_duplicate_hash", func(t *testing.T) {
				hash := "ml-dup-hash-" + tt.Name
				p := store.CreateMagicLinkTokenParams{
					ID:        nextID("ml-dup1"),
					TokenHash: hash,
					Email:     "ml1-" + tt.Name + "@example.com",
					IssuedAt:  now,
					ExpiresAt: now.Add(15 * time.Minute),
				}
				if _, err := s.CreateMagicLinkToken(ctx, p); err != nil {
					t.Fatalf("first CreateMagicLinkToken: %v", err)
				}

				p.ID = nextID("ml-dup2")
				p.Email = "ml2-" + tt.Name + "@example.com"
				_, err := s.CreateMagicLinkToken(ctx, p)
				if !errors.Is(err, store.ErrUniqueViolation) {
					t.Fatalf("expected ErrUniqueViolation on duplicate token_hash, got %v", err)
				}
			})

			t.Run("CreateOAuthToken_duplicate_hash", func(t *testing.T) {
				acc := mustCreateAccount(t, ctx, s, "oauth-dup-"+tt.Name+"@example.com")
				hash := "oauth-dup-hash-" + tt.Name
				p := store.CreateOAuthTokenParams{
					ID:        nextID("oauth-dup1"),
					AccountID: acc.ID,
					TokenHash: hash,
					Kind:      "access",
					IssuedAt:  now,
					ExpiresAt: now.Add(time.Hour),
				}
				if _, err := s.CreateOAuthToken(ctx, p); err != nil {
					t.Fatalf("first CreateOAuthToken: %v", err)
				}

				p.ID = nextID("oauth-dup2")
				_, err := s.CreateOAuthToken(ctx, p)
				if !errors.Is(err, store.ErrUniqueViolation) {
					t.Fatalf("expected ErrUniqueViolation on duplicate token_hash, got %v", err)
				}
			})

			t.Run("AddOrgMember_duplicate", func(t *testing.T) {
				org := mustCreateOrg(t, ctx, s, "dup-org-mem-"+tt.Name)
				acc := mustCreateAccount(t, ctx, s, "dup-mem-"+tt.Name+"@example.com")
				mustAddOrgMember(t, ctx, s, org.ID, acc.ID, "creator")

				// Second add of the same (org, account) pair must fail.
				err := s.AddOrgMember(ctx, store.AddOrgMemberParams{
					OrgID:     org.ID,
					AccountID: acc.ID,
					Role:      "member",
					CreatedAt: now,
				})
				if !errors.Is(err, store.ErrUniqueViolation) {
					t.Fatalf("expected ErrUniqueViolation on duplicate org_member, got %v", err)
				}
			})
		})
	}
}
