package store_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"jamsesh/internal/db/store"
)

// TestCreateAnonymousAccount_RoundTrip verifies that CreateAnonymousAccount
// inserts a row with is_anonymous=true and that GetAccountByID returns it
// with IsAnonymous=true and the synthetic email intact.
func TestCreateAnonymousAccount_RoundTrip(t *testing.T) {
	ctx := context.Background()

	for _, h := range stores(t) {
		h := h
		t.Run(h.name, func(t *testing.T) {
			s := h.open(t)

			accountID := nextID("anon")
			email := accountID + "@playground.local"

			acct, err := s.CreateAnonymousAccount(ctx, store.CreateAnonymousAccountParams{
				ID:          accountID,
				Email:       email,
				DisplayName: "amber-otter",
				CreatedAt:   time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("CreateAnonymousAccount: %v", err)
			}

			if acct.ID != accountID {
				t.Errorf("ID: want %q, got %q", accountID, acct.ID)
			}
			if acct.Email != email {
				t.Errorf("Email: want %q, got %q", email, acct.Email)
			}
			if acct.DisplayName != "amber-otter" {
				t.Errorf("DisplayName: want 'amber-otter', got %q", acct.DisplayName)
			}
			if !acct.IsAnonymous {
				t.Error("IsAnonymous: want true, got false")
			}
			if acct.GithubUserID != nil {
				t.Errorf("GithubUserID: want nil, got %v", acct.GithubUserID)
			}

			// GetAccountByID must return the same row with IsAnonymous=true.
			got, err := s.GetAccountByID(ctx, accountID)
			if err != nil {
				t.Fatalf("GetAccountByID: %v", err)
			}
			if !got.IsAnonymous {
				t.Error("GetAccountByID IsAnonymous: want true, got false")
			}
			if got.Email != email {
				t.Errorf("GetAccountByID Email: want %q, got %q", email, got.Email)
			}
		})
	}
}

// TestCreateAnonymousAccount_SyntheticEmail verifies that the synthetic email
// follows the expected format.
func TestCreateAnonymousAccount_SyntheticEmail(t *testing.T) {
	ctx := context.Background()

	for _, h := range stores(t) {
		h := h
		t.Run(h.name, func(t *testing.T) {
			s := h.open(t)

			accountID := nextID("anon")
			email := accountID + "@playground.local"

			acct, err := s.CreateAnonymousAccount(ctx, store.CreateAnonymousAccountParams{
				ID:          accountID,
				Email:       email,
				DisplayName: "blue-fox",
				CreatedAt:   time.Now().UTC(),
			})
			if err != nil {
				t.Fatalf("CreateAnonymousAccount: %v", err)
			}

			if !strings.HasSuffix(acct.Email, "@playground.local") {
				t.Errorf("email should end with @playground.local, got %q", acct.Email)
			}
		})
	}
}

// TestCreateAccount_IsAnonymous_DefaultsFalse verifies that the existing
// CreateAccount method sets is_anonymous=false by default (zero value).
func TestCreateAccount_IsAnonymous_DefaultsFalse(t *testing.T) {
	ctx := context.Background()

	for _, h := range stores(t) {
		h := h
		t.Run(h.name, func(t *testing.T) {
			s := h.open(t)

			acct := mustCreateAccount(t, ctx, s, nextID("regular")+"@example.com")
			if acct.IsAnonymous {
				t.Error("regular account should have IsAnonymous=false")
			}
		})
	}
}
