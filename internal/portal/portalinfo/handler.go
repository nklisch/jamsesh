// Package portalinfo implements the public GET /api/portal/info endpoint.
// The endpoint returns deploy-time portal configuration so the SPA can decide
// what to render at "/" for anonymous visitors before authentication completes.
// No authorization is required — the handler is intentionally public.
package portalinfo

import (
	"context"
	"fmt"
	"net/http"

	"jamsesh/internal/api/openapi"
)

// NoCacheMiddleware sets Cache-Control: no-store on every response. Mounted
// on GET /api/portal/info so deploy-time toggles (PlaygroundEnabled,
// LandingVariant) propagate immediately to all browsers and any
// intermediate cache without a stale-cache window.
// (gate-security-portalinfo-no-cachecontrol-no-store)
//
// "no-store" is stricter than "no-cache": "no-cache" still permits stored
// responses subject to revalidation; "no-store" prohibits caching the
// response at all, which is the desired behaviour for build-time config.
//
// The header is set BEFORE next.ServeHTTP so it's written before the
// strict-server handler calls WriteHeader — Go's net/http silently drops
// headers set after WriteHeader.
func NoCacheMiddleware(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Cache-Control", "no-store")
		next.ServeHTTP(w, r)
	})
}

// Handler implements the openapi.StrictServerInterface GetPortalInfo method.
// It holds a config snapshot captured at construction time and never re-reads
// the portal config — config is treated as immutable post-startup.
//
// Production wiring MUST go through NewHandler so the landing-variant string is
// validated against the OpenAPI enum at construction time. Test fixtures that
// exercise response-shape behaviour may continue to construct Handler{...}
// literals directly — the constructor is the only defense the type system
// gives us, but it's the right one for the production startup path.
type Handler struct {
	// PlaygroundEnabled mirrors config.Config.PlaygroundEnabled at startup.
	PlaygroundEnabled bool
	// LandingVariant mirrors config.Config.Landing.Variant at startup.
	// Valid values: "auto", "project", "login".
	LandingVariant string
}

// NewHandler constructs a *Handler with a validated LandingVariant. It returns
// a typed error (with the invalid value quoted) when landingVariant does not
// satisfy openapi.PortalInfoLandingVariant.Valid(); callers in cmd/portal
// surface the error at startup so a misconfigured deploy fails fast rather
// than silently emitting a non-enum value on /api/portal/info.
//
// The validator is the generated openapi.PortalInfoLandingVariant(s).Valid()
// (see internal/api/openapi/server.gen.go) so the source of truth stays the
// OpenAPI spec.
func NewHandler(playgroundEnabled bool, landingVariant string) (*Handler, error) {
	if !openapi.PortalInfoLandingVariant(landingVariant).Valid() {
		return nil, fmt.Errorf("portalinfo: invalid landing_variant %q", landingVariant)
	}
	return &Handler{
		PlaygroundEnabled: playgroundEnabled,
		LandingVariant:    landingVariant,
	}, nil
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
