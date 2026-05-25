// Package portalinfo implements the public GET /api/portal/info endpoint.
// The endpoint returns deploy-time portal configuration so the SPA can decide
// what to render at "/" for anonymous visitors before authentication completes.
// No authorization is required — the handler is intentionally public.
package portalinfo

import (
	"context"

	"jamsesh/internal/api/openapi"
)

// Handler implements the openapi.StrictServerInterface GetPortalInfo method.
// It holds a config snapshot captured at construction time and never re-reads
// the portal config — config is treated as immutable post-startup.
type Handler struct {
	// PlaygroundEnabled mirrors config.Config.PlaygroundEnabled at startup.
	PlaygroundEnabled bool
	// LandingVariant mirrors config.Config.Landing.Variant at startup.
	// Valid values: "auto", "project", "login".
	LandingVariant string
}

// GetPortalInfo handles GET /api/portal/info.
// It returns the portal's deploy-time playground and landing-variant state.
// No authentication is required; the endpoint is public by design.
func (h *Handler) GetPortalInfo(_ context.Context, _ openapi.GetPortalInfoRequestObject) (openapi.GetPortalInfoResponseObject, error) {
	return openapi.GetPortalInfo200JSONResponse{
		PlaygroundEnabled: h.PlaygroundEnabled,
		LandingVariant:    openapi.PortalInfoLandingVariant(h.LandingVariant),
	}, nil
}
