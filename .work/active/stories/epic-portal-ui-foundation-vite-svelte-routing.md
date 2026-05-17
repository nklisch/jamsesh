---
id: epic-portal-ui-foundation-vite-svelte-routing
kind: story
stage: implementing
tags: [ui]
parent: epic-portal-ui-foundation
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# UI Foundation — Vite, Svelte 5, Router, Embed

## Scope

Stand up the Vite + Svelte 5 + TypeScript toolchain inside the
existing `frontend/` directory (skeleton already in place from
`http-skeleton-openapi-bootstrap`), add a hand-rolled History-API
router, the app entry that imports `theme-bootstrap.ts` before
mount, and the Go embed wiring so the portal binary serves the SPA.

## Units delivered

- `frontend/vite.config.ts` — Svelte plugin + TS path resolution +
  dev proxy for `/api`, `/ws`, `/git`, `/mcp` → `http://localhost:8443`
- `frontend/svelte.config.js` — `vitePreprocess` for TS
- `frontend/tsconfig.json` — extended with Svelte + Vitest types
- `frontend/package.json` — add `svelte@^5.0.0`,
  `@sveltejs/vite-plugin-svelte@^4.0.0`, `vite@^5.0.0`,
  `svelte-check@^4.0.0`, `vitest@^2.0.0`,
  `@testing-library/svelte@^5.0.0`, `@testing-library/jest-dom`,
  `jsdom` (Vitest env)
- `frontend/index.html` — SPA root
- `frontend/src/main.ts` — entry; imports theme-bootstrap first
- `frontend/src/App.svelte` — top-level routing component
- `frontend/src/lib/router.svelte.ts` — History-API rune router
- `internal/portal/assets/assets.go` — `//go:embed` SPA serving
- `cmd/portal/main.go` (edit) — wire MountUI to assets handler
- `internal/portal/router/router.go` (edit) — add `MountUI
  http.Handler` to `router.Deps`, mount at `/` as catch-all
- `Makefile` (edit) — add `frontend-build` target running
  `cd frontend && npm install && npm run build`, plus update
  `generate` (or a `build` target) to run frontend-build before
  Go build

## Acceptance Criteria

- [ ] `cd frontend && npm install && npm run dev` brings up the
      Vite dev server on its default port (5173); accessing
      `/login` loads the bundle
- [ ] `cd frontend && npm run build` produces `frontend/dist/` with
      an `index.html` + bundled JS/CSS
- [ ] `npm run build && go build ./cmd/portal && ./portal` serves
      the built SPA at `/` with the History-API fallback (deep links
      like `/orgs/foo/sessions` return `index.html`)
- [ ] `router.svelte.ts` exposes `current` ($derived) and
      `navigate(path)` and updates reactively on popstate
- [ ] `theme-bootstrap.ts` (from the design-system feature) is
      imported BEFORE app.css and before Svelte mount — no FOUC on
      reload with `data-theme=dark` persisted
- [ ] Tests: `vitest run` is green for `router.svelte.ts` (route
      matching, params extraction, navigate updates current)

## Notes

- Coordination with design-system: this story expects
  `frontend/src/lib/styles/tokens.css`,
  `frontend/src/lib/styles/theme-bootstrap.ts`, and the component
  files to exist. If design-system has landed, just import. If not,
  this story's tests will skip those expectations and the
  agile-workflow:review step will note the integration gap.
- The `router.Deps.MountUI` field is added in this story. Other
  routes that already exist (`/api`, `/git`, `/mcp`, `/ws/*`) take
  precedence over the catch-all; the SPA mount must be the LAST
  route registered.
- `go:embed all:dist` requires `frontend/dist/` to exist at compile
  time. The Makefile must run frontend-build before go-build.
