# Mockups — feature-portal-visitor-entry-pages

Surfaces covered by this feature, with the mockup that drives each.

## Project landing (`/` under `JAMSESH_LANDING_VARIANT=project`)

**Mockup:** [`project-landing.html`](./project-landing.html)

The new component for jamsesh.dev — what unauthenticated visitors see at the
portal root when the deploy is configured as `project`. Hero with the
primary "Try the playground →" CTA, a three-card "what is it" grid, an
install-instructions disclosure, GitHub/Docs/Sign-in links throughout.

Adapted from the design language in
`.mockups/flows/playground-onboarding/01-prospect-landing.html` so the visual
voice carries across both surfaces.

For additional options, invoke `/ux-ui-design:screens
feature-portal-visitor-entry-pages` to generate the standard 4-option
exploration.

## Generic landing (`/` under `JAMSESH_LANDING_VARIANT=auto` with playground enabled)

**Mockup:** reuses
[`.mockups/screens/feature-portal-visitor-entry-pages/../../flows/playground-onboarding/01-prospect-landing.html`](../../flows/playground-onboarding/01-prospect-landing.html)
— the existing `/playground` (`PlaygroundLanding.svelte`) IS the generic
landing. Under `auto`, the SPA redirects unauthenticated `/` traffic to
`/playground`. No separate component is built.

## Login-only landing (`/` under `JAMSESH_LANDING_VARIANT=login`)

No mockup — today's bounce-to-`/login` behaviour, unchanged. The flag value
exists so operators can pin the legacy behaviour explicitly.

## Playground share-URL fix

The "404 on `/playground/<id>`" bug is a CLI fix (see
`story-fix-cli-playground-share-url`), not a portal-UI surface. No mockup —
the visual is the existing `/playground/s/<id>/join` (`JoinerPicker.svelte`)
already in the playground-onboarding flow.
