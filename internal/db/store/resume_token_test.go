package store_test

// resume_token_test.go — adapter tests for the ResumeTokenStore interface.
//
// Key invariants verified:
//   - CreateResumeToken + GetResumeTokenByHash round-trip.
//   - ConsumeResumeToken returns the winning row on first call.
//   - ConsumeResumeToken returns ErrNotFound on a second call (single-use).
//   - ConsumeResumeToken returns ErrNotFound when the token is already expired.
//   - ConsumeResumeToken returns ErrNotFound for an unknown token hash.

import (
	"context"
	"errors"
	"testing"
	"time"

	"jamsesh/internal/db/store"
)

// TestResumeTokenCreateAndGet verifies basic Create + GetByHash round-trip
// including all binding columns (session_id, org_id, account_id).
func TestResumeTokenCreateAndGet(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC().Truncate(time.Second)
			p := store.CreateResumeTokenParams{
				ID:        nextID("rt"),
				TokenHash: "hash-create-get-" + tt.Name,
				SessionID: "sess-01",
				OrgID:     "org-01",
				AccountID: "acc-01",
				IssuedAt:  now,
				ExpiresAt: now.Add(5 * time.Minute),
				UsedAt:    nil,
			}

			created, err := s.CreateResumeToken(ctx, p)
			if err != nil {
				t.Fatalf("CreateResumeToken: %v", err)
			}
			if created.ID != p.ID {
				t.Errorf("ID: got %q, want %q", created.ID, p.ID)
			}
			if created.SessionID != p.SessionID {
				t.Errorf("SessionID: got %q, want %q", created.SessionID, p.SessionID)
			}
			if created.OrgID != p.OrgID {
				t.Errorf("OrgID: got %q, want %q", created.OrgID, p.OrgID)
			}
			if created.AccountID != p.AccountID {
				t.Errorf("AccountID: got %q, want %q", created.AccountID, p.AccountID)
			}
			if created.UsedAt != nil {
				t.Errorf("UsedAt: expected nil, got %v", created.UsedAt)
			}

			fetched, err := s.GetResumeTokenByHash(ctx, p.TokenHash)
			if err != nil {
				t.Fatalf("GetResumeTokenByHash: %v", err)
			}
			if fetched.ID != p.ID {
				t.Errorf("fetched ID: got %q, want %q", fetched.ID, p.ID)
			}
			if fetched.SessionID != p.SessionID {
				t.Errorf("fetched SessionID: got %q, want %q", fetched.SessionID, p.SessionID)
			}
		})
	}
}

// TestResumeTokenConsumeWinner verifies that ConsumeResumeToken returns the
// full token row on first call (the "winner" signal) and sets used_at.
func TestResumeTokenConsumeWinner(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC().Truncate(time.Second)
			hash := "hash-consume-winner-" + tt.Name
			_, err := s.CreateResumeToken(ctx, store.CreateResumeTokenParams{
				ID:        nextID("rt"),
				TokenHash: hash,
				SessionID: "sess-win",
				OrgID:     "org-win",
				AccountID: "acc-win",
				IssuedAt:  now,
				ExpiresAt: now.Add(5 * time.Minute),
				UsedAt:    nil,
			})
			if err != nil {
				t.Fatalf("CreateResumeToken: %v", err)
			}

			usedAt := now.Add(time.Minute)
			winner, err := s.ConsumeResumeToken(ctx, store.ConsumeResumeTokenParams{
				TokenHash: hash,
				UsedAt:    &usedAt,
				Now:       now,
			})
			if err != nil {
				t.Fatalf("ConsumeResumeToken (first): %v", err)
			}
			if winner.TokenHash != hash {
				t.Errorf("winner TokenHash: got %q, want %q", winner.TokenHash, hash)
			}
			if winner.SessionID != "sess-win" {
				t.Errorf("winner SessionID: got %q, want %q", winner.SessionID, "sess-win")
			}
			if winner.UsedAt == nil {
				t.Error("winner UsedAt: expected non-nil, got nil")
			}
		})
	}
}

// TestResumeTokenSingleUse verifies that the second ConsumeResumeToken on the
// same token returns ErrNotFound (already consumed → used_at IS NULL fails).
func TestResumeTokenSingleUse(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC().Truncate(time.Second)
			hash := "hash-single-use-" + tt.Name
			_, err := s.CreateResumeToken(ctx, store.CreateResumeTokenParams{
				ID:        nextID("rt"),
				TokenHash: hash,
				SessionID: "sess-su",
				OrgID:     "org-su",
				AccountID: "acc-su",
				IssuedAt:  now,
				ExpiresAt: now.Add(5 * time.Minute),
				UsedAt:    nil,
			})
			if err != nil {
				t.Fatalf("CreateResumeToken: %v", err)
			}

			usedAt := now.Add(time.Minute)
			cp := store.ConsumeResumeTokenParams{
				TokenHash: hash,
				UsedAt:    &usedAt,
				Now:       now,
			}

			if _, err := s.ConsumeResumeToken(ctx, cp); err != nil {
				t.Fatalf("first ConsumeResumeToken: %v", err)
			}

			// Second consume: token already used — must return ErrNotFound.
			_, err = s.ConsumeResumeToken(ctx, cp)
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("second ConsumeResumeToken: expected ErrNotFound, got %v", err)
			}
		})
	}
}

// TestResumeTokenExpiredNotConsumed verifies that ConsumeResumeToken on an
// already-expired token returns ErrNotFound (expires_at > now fails).
func TestResumeTokenExpiredNotConsumed(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC().Truncate(time.Second)
			hash := "hash-expired-" + tt.Name

			// Issue a token that expired 1 minute ago.
			pastExpiry := now.Add(-time.Minute)
			_, err := s.CreateResumeToken(ctx, store.CreateResumeTokenParams{
				ID:        nextID("rt"),
				TokenHash: hash,
				SessionID: "sess-exp",
				OrgID:     "org-exp",
				AccountID: "acc-exp",
				IssuedAt:  now.Add(-2 * time.Minute),
				ExpiresAt: pastExpiry,
				UsedAt:    nil,
			})
			if err != nil {
				t.Fatalf("CreateResumeToken: %v", err)
			}

			usedAt := now
			_, err = s.ConsumeResumeToken(ctx, store.ConsumeResumeTokenParams{
				TokenHash: hash,
				UsedAt:    &usedAt,
				Now:       now, // now > expires_at → WHERE expires_at > now is false
			})
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("ConsumeResumeToken on expired token: expected ErrNotFound, got %v", err)
			}
		})
	}
}

// TestResumeTokenUnknownHash verifies that ConsumeResumeToken with an unknown
// token hash returns ErrNotFound.
func TestResumeTokenUnknownHash(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC()
			usedAt := now
			_, err := s.ConsumeResumeToken(ctx, store.ConsumeResumeTokenParams{
				TokenHash: "no-such-hash-" + tt.Name,
				UsedAt:    &usedAt,
				Now:       now,
			})
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("ConsumeResumeToken unknown hash: expected ErrNotFound, got %v", err)
			}
		})
	}
}

// TestResumeTokenGetNotFound verifies that GetResumeTokenByHash on an unknown
// hash returns ErrNotFound.
func TestResumeTokenGetNotFound(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			_, err := s.GetResumeTokenByHash(ctx, "no-such-hash-get-"+tt.Name)
			if !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("GetResumeTokenByHash missing: expected ErrNotFound, got %v", err)
			}
		})
	}
}

// TestResumeTokenUniqueHash verifies that inserting two tokens with the same
// hash returns ErrUniqueViolation.
func TestResumeTokenUniqueHash(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)

			now := time.Now().UTC().Truncate(time.Second)
			hash := "hash-unique-" + tt.Name
			p := store.CreateResumeTokenParams{
				ID:        nextID("rt"),
				TokenHash: hash,
				SessionID: "sess-dup",
				OrgID:     "org-dup",
				AccountID: "acc-dup",
				IssuedAt:  now,
				ExpiresAt: now.Add(5 * time.Minute),
			}

			if _, err := s.CreateResumeToken(ctx, p); err != nil {
				t.Fatalf("first CreateResumeToken: %v", err)
			}

			p.ID = nextID("rt") // different primary key, same hash
			_, err := s.CreateResumeToken(ctx, p)
			if !errors.Is(err, store.ErrUniqueViolation) {
				t.Fatalf("duplicate hash: expected ErrUniqueViolation, got %v", err)
			}
		})
	}
}
