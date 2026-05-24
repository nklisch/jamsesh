// Package playground provides startup provisioning and configuration for the
// ephemeral anonymous playground subsystem. When JAMSESH_PLAYGROUND_ENABLED=true,
// the portal seeds a reserved system-owned org at boot (idempotent) and playground
// REST routes accept traffic. When false, those routes return 503.
package playground

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"time"

	"jamsesh/internal/db/store"
)

// ReservedOrgSlug is the hardcoded slug for the system-owned playground
// org. Per the parent epic's strategic decision, this is NOT configurable.
// Docs, support material, observability dashboards, and pre-receive checks
// can hard-reference org:playground without env-var lookup.
const ReservedOrgSlug = "playground"

// ReservedOrgID is the deterministic ID for the playground org.
// Using a deterministic ID lets observability dashboards hard-reference
// it without env-var lookup.
const ReservedOrgID = "org_playground"

// ReservedOrgName is the human-readable name.
const ReservedOrgName = "Playground"

// ErrReservedSlugConflict signals that an unprotected org claims the
// reserved playground slug. The portal refuses to start in this state.
// The operator must either rename the existing org or disable playground.
var ErrReservedSlugConflict = errors.New("reserved slug conflict")

// ProvisionReservedOrg ensures the reserved `playground` org row exists
// when playground is enabled. Idempotent: safe to call on every boot.
//
// Behavior:
//   - If no org with slug "playground" exists: creates a protected org row.
//   - If a protected org with slug "playground" exists: no-op, returns nil.
//   - If an UNPROTECTED org with slug "playground" exists (operator had
//     a real org by that name before enabling this feature): refuses to start.
//     Returns ErrReservedSlugConflict so cmd/portal/main.go can log a clear
//     actionable error and exit 1.
//
// The conflict check fires on EVERY boot — not just first provisioning —
// so if the protected playground org is later renamed and a regular org
// takes the slug, the next boot catches it.
func ProvisionReservedOrg(ctx context.Context, s store.Store, now time.Time, logger *slog.Logger) error {
	existing, err := s.GetOrgBySlug(ctx, ReservedOrgSlug)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		return fmt.Errorf("lookup existing %s org: %w", ReservedOrgSlug, err)
	}

	if err == nil {
		// Org exists. Check protection flag.
		if !existing.OrgProtected {
			return fmt.Errorf("%w: an unprotected org with slug %q exists (id=%s); "+
				"rename it before enabling playground",
				ErrReservedSlugConflict, ReservedOrgSlug, existing.ID)
		}
		// Already provisioned; idempotent no-op.
		logger.Info("playground org already provisioned", "org_id", existing.ID)
		return nil
	}

	// No org with that slug exists; provision it.
	org, err := s.CreateProtectedOrg(ctx, store.CreateProtectedOrgParams{
		ID:        ReservedOrgID,
		Name:      ReservedOrgName,
		Slug:      ReservedOrgSlug,
		CreatedAt: now,
	})
	if err != nil {
		return fmt.Errorf("create %s org: %w", ReservedOrgSlug, err)
	}
	logger.Info("playground org provisioned", "org_id", org.ID)
	return nil
}
