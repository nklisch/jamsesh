---
id: gate-tests-projectlanding-hardcoded-version-string
kind: story
stage: review
tags: [testing, ui, cleanup]
parent: feature-spa-bootstrap-hygiene
depends_on: []
release_binding: null
gate_origin: tests
created: 2026-05-25
updated: 2026-05-25
---

# ProjectLanding colophon hard-codes `v0.4.0`

## Priority
Low

## Spec reference
Item: `story-portal-visitor-entry-pages-spa-landing`
Test `ProjectLanding.test.ts:97` asserts the literal `v0.4.0` string.

## Gap type
Drift pressure — every release bump rots this test.

## Location
`frontend/src/lib/screens/ProjectLanding.svelte:118` —
`jamsesh / Apache-2.0 / v0.4.0 / 2026`. Test at
`ProjectLanding.test.ts:97` matches the literal. After bumping the
release this assertion goes stale.

## Remediation direction
Make the version a build-time constant (Vite `define`) and assert "the
version string is present and matches a semver-ish pattern" rather than
the literal. Or fold a ProjectLanding version-bump into `release-bump.sh`.

## Implementation notes

- `frontend/vite.config.ts`:
  - Added `import pkg from './package.json' with { type: 'json' };` (Node
    18+ / Vite 5 native import-attributes syntax).
  - Added `define: { __APP_VERSION__: JSON.stringify(\`v${pkg.version}\`) }`
    so the string literal `__APP_VERSION__` is substituted at bundle/test
    time with e.g. `"v0.0.1"`.
- `frontend/src/vite-env.d.ts` (new file) declares
  `declare const __APP_VERSION__: string;` so TypeScript / svelte-check do
  not flag the identifier.
- `frontend/src/lib/screens/ProjectLanding.svelte`: replaced the literal
  `v0.4.0` colophon string with `{__APP_VERSION__}` in the meta div.
- `frontend/src/lib/screens/ProjectLanding.test.ts`: the colophon
  assertion now matches a semver-shape regex
  `/jamsesh \/ Apache-2\.0 \/ v?\d+\.\d+\.\d+ \/ 2026/i` instead of the
  literal. Future `package.json` bumps require no test edit.
- Vitest also applies the Vite `define` block during test compilation, so
  the test sees the real package.json version (currently `v0.0.1`).

Verified:
- `npm test -- --run ProjectLanding.test.ts` → 18 passed.
- `npm run check` → 0 errors (1 pre-existing unrelated warning).
- `npm run build` → succeeds, 226 modules transformed.
