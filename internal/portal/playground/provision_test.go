package playground_test

import (
	"context"
	"errors"
	"log/slog"
	"os"
	"testing"
	"time"

	"jamsesh/internal/db/store"
	"jamsesh/internal/db/store/storetest"
	"jamsesh/internal/portal/playground"
)

// stores is a one-line wrapper over storetest.Stores so existing call sites
// in this package read tt.Name / tt.Open(t) without the package-qualified
// type name in the type literal.
func stores(t *testing.T) []storetest.DialectHarness {
	t.Helper()
	return storetest.Stores(t)
}

// discardLogger returns a slog.Logger that drops all output.
func discardLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.NewFile(0, os.DevNull), &slog.HandlerOptions{}))
}

// TestProvisionReservedOrg_NoExistingOrg verifies the first-boot path:
// when no org with slug "playground" exists, ProvisionReservedOrg creates
// a protected org row with the deterministic ID and correct fields.
func TestProvisionReservedOrg_NoExistingOrg(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)
			logger := discardLogger()
			now := time.Now().UTC()

			err := playground.ProvisionReservedOrg(ctx, s, now, logger)
			if err != nil {
				t.Fatalf("ProvisionReservedOrg: unexpected error: %v", err)
			}

			// Verify the org was created with the correct fields.
			org, err := s.GetOrgBySlug(ctx, playground.ReservedOrgSlug)
			if err != nil {
				t.Fatalf("GetOrgBySlug(%q): %v", playground.ReservedOrgSlug, err)
			}
			if org.ID != playground.ReservedOrgID {
				t.Errorf("org.ID = %q, want %q", org.ID, playground.ReservedOrgID)
			}
			if org.Name != playground.ReservedOrgName {
				t.Errorf("org.Name = %q, want %q", org.Name, playground.ReservedOrgName)
			}
			if org.Slug != playground.ReservedOrgSlug {
				t.Errorf("org.Slug = %q, want %q", org.Slug, playground.ReservedOrgSlug)
			}
			if !org.OrgProtected {
				t.Error("org.OrgProtected = false, want true")
			}
			if org.SessionInvitePolicy != "open" {
				t.Errorf("org.SessionInvitePolicy = %q, want %q", org.SessionInvitePolicy, "open")
			}
		})
	}
}

// TestProvisionReservedOrg_AlreadyProvisioned verifies the idempotent path:
// when a protected org with slug "playground" already exists, ProvisionReservedOrg
// is a no-op and returns nil.
func TestProvisionReservedOrg_AlreadyProvisioned(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)
			logger := discardLogger()
			now := time.Now().UTC()

			// First boot: provision the org.
			if err := playground.ProvisionReservedOrg(ctx, s, now, logger); err != nil {
				t.Fatalf("first ProvisionReservedOrg: %v", err)
			}

			// Second boot: must be idempotent.
			if err := playground.ProvisionReservedOrg(ctx, s, now.Add(time.Minute), logger); err != nil {
				t.Fatalf("second ProvisionReservedOrg: unexpected error: %v", err)
			}

			// Still exactly one org row.
			org, err := s.GetOrgBySlug(ctx, playground.ReservedOrgSlug)
			if err != nil {
				t.Fatalf("GetOrgBySlug after second provision: %v", err)
			}
			if org.ID != playground.ReservedOrgID {
				t.Errorf("org.ID = %q, want %q", org.ID, playground.ReservedOrgID)
			}
		})
	}
}

// TestProvisionReservedOrg_UnprotectedSlugConflict verifies the conflict path:
// when an unprotected org already holds slug "playground", ProvisionReservedOrg
// refuses to start and returns ErrReservedSlugConflict.
func TestProvisionReservedOrg_UnprotectedSlugConflict(t *testing.T) {
	for _, tt := range stores(t) {
		tt := tt
		t.Run(tt.Name, func(t *testing.T) {
			ctx := context.Background()
			s := tt.Open(t)
			logger := discardLogger()
			now := time.Now().UTC()

			// Pre-condition: an operator-created (unprotected) org with the reserved slug.
			_, err := s.CreateOrg(ctx, store.CreateOrgParams{
				ID:        "org-existing-unprotected",
				Name:      "Existing Playground",
				Slug:      playground.ReservedOrgSlug,
				CreatedAt: now,
			})
			if err != nil {
				t.Fatalf("CreateOrg (pre-existing): %v", err)
			}

			// ProvisionReservedOrg must refuse and return ErrReservedSlugConflict.
			err = playground.ProvisionReservedOrg(ctx, s, now, logger)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !errors.Is(err, playground.ErrReservedSlugConflict) {
				t.Errorf("expected ErrReservedSlugConflict, got: %v", err)
			}
		})
	}
}
