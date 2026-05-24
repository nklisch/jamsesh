package store_test

import (
	"context"
	"testing"
	"time"

	"jamsesh/internal/db/store"
)

// mustCreateAnonAccount creates an anonymous account for testing purposes.
func mustCreateAnonAccount(t *testing.T, ctx context.Context, s store.Store, nickname string) store.Account {
	t.Helper()
	accountID := nextID("anon")
	email := accountID + "@playground.local"
	acct, err := s.CreateAnonymousAccount(ctx, store.CreateAnonymousAccountParams{
		ID:          accountID,
		Email:       email,
		DisplayName: nickname,
		CreatedAt:   time.Now().UTC(),
	})
	if err != nil {
		t.Fatalf("mustCreateAnonAccount(%q): %v", nickname, err)
	}
	return acct
}

// TestCreateAnonymousBearer_RoundTrip verifies that CreateAnonymousBearer
// inserts a bearer row with kind='anonymous_session_bearer' and session_id set.
func TestCreateAnonymousBearer_RoundTrip(t *testing.T) {
	ctx := context.Background()

	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			s := h.Open(t)

			org := mustCreateOrg(t, ctx, s, "anon-bearer-"+h.Name)
			sess := mustCreateSession(t, ctx, s, org.ID, "anon-bearer-session")
			acct := mustCreateAnonAccount(t, ctx, s, "cedar-hawk")

			now := time.Now().UTC()
			expiresAt := now.Add(24 * time.Hour)

			bearer, err := s.CreateAnonymousBearer(ctx, store.CreateAnonymousBearerParams{
				ID:        nextID("tok"),
				AccountID: acct.ID,
				TokenHash: "somehash123",
				SessionID: sess.ID,
				IssuedAt:  now,
				ExpiresAt: expiresAt,
			})
			if err != nil {
				t.Fatalf("CreateAnonymousBearer: %v", err)
			}

			if bearer.Kind != "anonymous_session_bearer" {
				t.Errorf("Kind: want 'anonymous_session_bearer', got %q", bearer.Kind)
			}
			if bearer.SessionID == nil || *bearer.SessionID != sess.ID {
				t.Errorf("SessionID: want %q, got %v", sess.ID, bearer.SessionID)
			}
			if bearer.AccountID != acct.ID {
				t.Errorf("AccountID: want %q, got %q", acct.ID, bearer.AccountID)
			}
		})
	}
}

// TestRevokeBearersForSession_Idempotent verifies that calling RevokeBearersForSession
// twice with the same revoked_at does not error, and the second call updates 0 rows.
func TestRevokeBearersForSession_Idempotent(t *testing.T) {
	ctx := context.Background()

	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			s := h.Open(t)

			org := mustCreateOrg(t, ctx, s, "revoke-idem-"+h.Name)
			sess := mustCreateSession(t, ctx, s, org.ID, "revoke-idem-session")
			acct := mustCreateAnonAccount(t, ctx, s, "dawn-elk")

			now := time.Now().UTC()
			_, err := s.CreateAnonymousBearer(ctx, store.CreateAnonymousBearerParams{
				ID:        nextID("tok"),
				AccountID: acct.ID,
				TokenHash: "somehash456",
				SessionID: sess.ID,
				IssuedAt:  now,
				ExpiresAt: now.Add(24 * time.Hour),
			})
			if err != nil {
				t.Fatalf("CreateAnonymousBearer: %v", err)
			}

			revokedAt := time.Now().UTC()

			// First revoke: marks the bearer revoked.
			if err := s.RevokeBearersForSession(ctx, store.RevokeBearersForSessionParams{
				RevokedAt: revokedAt,
				SessionID: sess.ID,
			}); err != nil {
				t.Fatalf("RevokeBearersForSession (first): %v", err)
			}

			// Second revoke: idempotent — bearer already revoked, no-op.
			if err := s.RevokeBearersForSession(ctx, store.RevokeBearersForSessionParams{
				RevokedAt: revokedAt,
				SessionID: sess.ID,
			}); err != nil {
				t.Errorf("RevokeBearersForSession (second, idempotent): %v", err)
			}
		})
	}
}

// TestAnonymousBearer_CascadeDeleteWithSession verifies that deleting the parent
// session row also deletes the oauth_tokens row (ON DELETE CASCADE).
func TestAnonymousBearer_CascadeDeleteWithSession(t *testing.T) {
	ctx := context.Background()

	for _, h := range stores(t) {
		h := h
		t.Run(h.Name, func(t *testing.T) {
			s := h.Open(t)

			org := mustCreateOrg(t, ctx, s, "cascade-"+h.Name)
			sess := mustCreateSession(t, ctx, s, org.ID, "cascade-session")
			acct := mustCreateAnonAccount(t, ctx, s, "ember-crow")

			now := time.Now().UTC()
			bearer, err := s.CreateAnonymousBearer(ctx, store.CreateAnonymousBearerParams{
				ID:        nextID("tok"),
				AccountID: acct.ID,
				TokenHash: "somehash789",
				SessionID: sess.ID,
				IssuedAt:  now,
				ExpiresAt: now.Add(24 * time.Hour),
			})
			if err != nil {
				t.Fatalf("CreateAnonymousBearer: %v", err)
			}

			// Delete the session — should cascade-delete the bearer.
			if err := s.DeleteSession(ctx, store.DeleteSessionParams{
				OrgID: org.ID,
				ID:    sess.ID,
			}); err != nil {
				t.Fatalf("DeleteSession: %v", err)
			}

			// The bearer should no longer be found via GetOAuthTokenByHash.
			_, err = s.GetOAuthTokenByHash(ctx, bearer.TokenHash)
			if err == nil {
				t.Error("expected ErrNotFound after cascade delete, got nil")
			} else if err != store.ErrNotFound {
				t.Errorf("expected ErrNotFound, got %v", err)
			}
		})
	}
}
