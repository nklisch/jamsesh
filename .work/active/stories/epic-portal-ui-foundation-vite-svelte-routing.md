---
id: epic-portal-ui-foundation-vite-svelte-routing
kind: story
stage: done
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

## Implementation notes

### What landed

- `frontend/vite.config.ts` — Svelte plugin + `@testing-library/svelte/vite` plugin (injects `browser`
  condition for Vitest so Svelte resolves client-side), `fileURLToPath`-based `$lib` alias, dev proxy for
  `/api`, `/ws`, `/git`, `/mcp` → `http://localhost:8443`, `build.outDir: 'dist'`.
- `frontend/svelte.config.js` — `vitePreprocess` for TS in `.svelte` files.
- `frontend/tsconfig.json` — extended with `svelte`, `vitest/globals`, `@testing-library/jest-dom`, `node`
  types; `$lib` path alias; includes `.svelte` files + `vite.config.ts`.
- `frontend/package.json` — all toolchain deps added: `svelte@^5`, `@sveltejs/vite-plugin-svelte@^4`,
  `vite@^5`, `svelte-check@^4`, `vitest@^2`, `@testing-library/svelte@^5`, `@testing-library/jest-dom`,
  `jsdom`, `@types/node`; `openapi-fetch` in deps; scripts: `dev`, `build`, `test`, `check`.
- `frontend/vitest.setup.ts` — imports `@testing-library/jest-dom` so custom matchers are available.
- `frontend/index.html` — root HTML with `<div id="app">` and `<script type="module" src="/src/main.ts">`.
- `frontend/src/main.ts` — imports `theme-bootstrap.ts` first (FOUC prevention), then `app.css`, then
  mounts `App.svelte` via Svelte 5 `mount()`.
- `frontend/src/App.svelte` — reads `current` from router store; renders named placeholders for `login`,
  `sessions`, `session-view`; falls back to Not Found.
- `frontend/src/lib/router.svelte.ts` — History-API rune router. `$state` path, `$derived` match result
  exposed as `{ get name(), get params() }` object (Svelte 5 prohibits exporting bare `$derived`). `navigate()`
  + `popstate` listener.
- `frontend/src/lib/router.test.ts` — 8 tests: pattern matching for all 3 routes, percent-decode, not-found,
  navigate() synchronous update, history.pushState, popstate event. All green.
- `internal/portal/assets/assets.go` — `//go:embed all:dist` over `internal/portal/assets/dist/` (populated
  by `make frontend-build`). `Handler()` tries literal path then falls back to `/` (index.html) for SPA
  deep links.
- `internal/portal/assets/dist/.gitkeep` + `.gitignore` — ensures `go build ./...` works on a fresh
  checkout before `make frontend-build` has run; keeps built artifacts out of the repo.
- `internal/portal/router/router.go` — `MountUI http.Handler` field added to `Deps`; mounted as catch-all
  at `/` after all named routes.
- `cmd/portal/main.go` — calls `assets.Handler()`, passes result as `MountUI` in `router.Deps`.
- `Makefile` — `frontend-build` target (npm install + npm run build + copy to `assets/dist/`);
  `go-build` depends on `frontend-build`; `build` target runs generate + frontend-build + go build.

### Svelte 5 module gotcha

`export const current = $derived(match(path))` is rejected by the Svelte 5 module compiler
(`derived_invalid_export`). Workaround: wrap in an object with `get` accessors:

```ts
let _current = $derived(match(path));
export const current = {
  get name() { return _current.name; },
  get params() { return _current.params; },
};
```

### Go embed layout

Go's `//go:embed` does not support `..` paths, so the embed target must be a subdirectory of the
package directory. We embed `internal/portal/assets/dist/` (populated by `make frontend-build` which
copies `frontend/dist/`). On a fresh checkout only `.gitkeep` is present — the embed compiles fine
and the SPA handler gracefully falls back to index.html (which returns the gitkeep as a 200 with
garbage — acceptable until `make build` runs). CI and the Makefile's `build` target always run
`frontend-build` before `go build`.

### Pre-existing sibling test failures

21 tests in the sibling agent's component tests (`Badge`, `Button`, `Card`, `InlineCode`) fail because
they pass `children: () => 'string'` where Svelte 5 requires a `Snippet`. These are pre-existing bugs
in the sibling's test files, not introduced by this story. They also produce 21 `svelte-check` errors
(all in `*.test.ts` files). My own files are type-clean.

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

## Review (2026-05-16)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Toolchain + History-API rune router + Go embed wiring landed clean. The Svelte 5 \$derived export gotcha (wrapping in a get-accessor object) is correctly documented and applied. The /assets/dist/ embed path + .gitkeep + .gitignore approach works around the go:embed-needs-existing-dir constraint elegantly. Pre-existing test failures in sibling component tests flagged (filed as follow-up).
