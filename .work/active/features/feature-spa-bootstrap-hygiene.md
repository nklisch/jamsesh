---
id: feature-spa-bootstrap-hygiene
kind: feature
stage: implementing
tags: [security, portal, ui, csp, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-25
updated: 2026-05-25
---

# SPA bootstrap hygiene

## Brief

Close three loose ends on the SPA bootstrap surface — the unwired CSP
report endpoint, missing `Cache-Control: no-store` on `/api/portal/info`,
and a release-coupled hardcoded version string in `ProjectLanding`. All
three sit at the public unauthenticated bootstrap boundary and have the
same shape: a header or endpoint that should already exist, or a literal
that should be sourced from build-time config.

Bounded — no architectural shift, no foundation-doc impact. The CSP
endpoint, the portal-info cache headers, and the version constant land
side-by-side in the SPA-bootstrap shell.

## Member stories

- `bug-csp-report-endpoint-not-wired` —
  add `POST /_csp-report` route that logs JSON body at warn and returns 204
- `gate-security-portalinfo-no-cachecontrol-no-store` —
  set `Cache-Control: no-store` on `/api/portal/info` so deploy-time
  toggles propagate
- `gate-tests-projectlanding-hardcoded-version-string` —
  source the colophon version from a Vite build-time constant; assert a
  semver-shape pattern rather than the literal

## Approach (high level)

All three are independent. The CSP-report route is the largest piece —
new handler in `internal/portal/router/` with a structured-log key. The
other two are small surgical edits.

## Design decisions

- **Independence of stories**: All three stories are fully independent —
  no shared code paths, no sequencing needed. Implement in parallel.
- **CSP handler location**: Implemented directly in `router.go` as a
  package-private handler function (same pattern as `healthz`), registered
  as a top-level unauthenticated route. No new package; the handler is
  three lines.
- **Cache-Control injection method**: Applied via a route-local middleware
  wrapper on the `/api/portal/info` route registration site in the API
  mount hook (inside `cmd/portal/main.go`'s API mount). This preserves the
  generated `GetPortalInfo200JSONResponse.VisitGetPortalInfoResponse`
  without modification — the header is written before the generated handler
  sets `Content-Type` and calls `WriteHeader`. Simpler than a custom
  response-object wrapper.
- **Version constant source**: Vite `define` block in `vite.config.ts`
  reads version from `package.json` at build time (`__APP_VERSION__`).
  `ProjectLanding.svelte` references `__APP_VERSION__` as a string constant.
  The test asserts a semver-shape regex (`/v?\d+\.\d+\.\d+/`) rather than
  a literal. `frontend/package.json` version field is the canonical source
  and is already bumped by `scripts/release-bump.sh` convention (operator
  can add it there; initially set to match the Go release tag).

## Architectural choice

All three units follow the "smallest surgical edit" path — no new packages,
no new abstractions, no generated contract changes. The CSP handler and
Cache-Control middleware fit the existing chi router pattern exactly. The
Vite `define` is the standard Vite mechanism for build-time constants; it
replaces the literal at bundle time, so tests and production builds both
work without special casing.

Rejected: a separate `cspreport` package — unnecessary indirection for a
three-line handler. Rejected: modifying the oapi-codegen generated
`VisitGetPortalInfoResponse` — generated code must not be hand-edited.

## Implementation Units

### Unit 1: `POST /_csp-report` handler
**File**: `internal/portal/router/router.go`
**Story**: `bug-csp-report-endpoint-not-wired`

```go
// cspReport handles POST /_csp-report.
// Browsers POST CSP violation reports here when the Report-Only or enforced
// CSP policy includes a report-uri directive. The endpoint reads the JSON body,
// logs it at warn level with key "csp_violation", and returns 204 — browsers
// do not use the response body. No authentication is required; browsers send
// reports without credentials.
func cspReport(w http.ResponseWriter, r *http.Request) {
    var report map[string]any
    if err := json.NewDecoder(r.Body).Decode(&report); err != nil {
        // Malformed body: still return 204 — do not encourage browser retries.
        slog.WarnContext(r.Context(), "csp_violation", "parse_err", err)
        w.WriteHeader(http.StatusNoContent)
        return
    }
    slog.WarnContext(r.Context(), "csp_violation", "report", report)
    w.WriteHeader(http.StatusNoContent)
}
```

Registration in `New()` after the `healthz` route, before the `/api` group:
```go
r.Post("/_csp-report", cspReport)
```

**Implementation Notes**:
- `slog` is already in stdlib; add `"log/slog"` to the import block in
  `router.go`.
- Body is capped by the global `BodyLimit` only on `/api/*` routes; the
  `/_csp-report` route is top-level. Add a local `http.MaxBytesReader` guard
  (64 KiB) to bound memory usage from a malicious body. CSP reports are
  typically 1–4 KiB; 64 KiB is generous.
- Return 204 even on parse error — see comment above.

**Acceptance Criteria**:
- [ ] `POST /_csp-report` with a valid JSON body returns 204.
- [ ] A `slog` warn-level record with key `csp_violation` is emitted.
- [ ] `POST /_csp-report` with a malformed body still returns 204 (no 400).
- [ ] `GET /_csp-report` returns 405 (chi MethodNotAllowed envelope).
- [ ] The route requires no `Authorization` header.

---

### Unit 2: `Cache-Control: no-store` on `/api/portal/info`
**File**: `internal/portal/portalinfo/handler.go`
**Story**: `gate-security-portalinfo-no-cachecontrol-no-store`

The `GetPortalInfo200JSONResponse.VisitGetPortalInfoResponse` is generated
code and must not be edited. Instead, `Cache-Control: no-store` is set
on the `http.ResponseWriter` before the strict handler wrapper calls
`VisitGetPortalInfoResponse`. The clean insertion point is at the API mount
in `cmd/portal/main.go` — wrap the `GetPortalInfo` route registration with
a one-line middleware. However, to keep the logic testable and co-located
with the handler, add a `NoCacheMiddleware` helper in the `portalinfo`
package and apply it at the mount site.

```go
// NoCacheMiddleware sets Cache-Control: no-store on every response.
// Applied to GET /api/portal/info so that deploy-time toggles
// (playground_enabled, landing_variant) propagate immediately to all
// browsers and CDNs without a stale-cache window.
func NoCacheMiddleware(next http.Handler) http.Handler {
    return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
        w.Header().Set("Cache-Control", "no-store")
        next.ServeHTTP(w, r)
    })
}
```

Mount site in `cmd/portal/main.go` (inside the API mount hook):
```go
r.With(portalinfo.NoCacheMiddleware).Get("/portal/info", apiWrapper.GetPortalInfo)
```

**Implementation Notes**:
- The `r.With(...).Get(...)` chi pattern applies the middleware only to
  this single route — no risk of accidentally caching other endpoints.
- The header is set before `next.ServeHTTP`, which calls
  `VisitGetPortalInfoResponse(w)` → `w.WriteHeader(200)`. Go's
  `net/http` requires headers to be set before `WriteHeader`; this ordering
  is correct.
- `"no-store"` is the correct directive: `"no-cache"` still allows
  revalidation; `"no-store"` prohibits caching the response at all, which
  is the desired behavior for deploy-time config.

**Acceptance Criteria**:
- [ ] `GET /api/portal/info` response includes `Cache-Control: no-store`.
- [ ] The response body is still valid JSON with `playground_enabled` and
  `landing_variant` fields.
- [ ] The existing handler tests (`TestGetPortalInfo`) continue to pass
  with the new middleware in the test router.

---

### Unit 3: Vite build-time version constant in ProjectLanding
**Files**:
- `frontend/vite.config.ts` — add `define: { __APP_VERSION__: JSON.stringify(version) }`
- `frontend/src/lib/screens/ProjectLanding.svelte` — replace `v0.4.0` literal
- `frontend/src/lib/screens/ProjectLanding.test.ts` — update assertion to semver regex
**Story**: `gate-tests-projectlanding-hardcoded-version-string`

`vite.config.ts` change:
```typescript
import pkg from './package.json' with { type: 'json' };

export default defineConfig({
  define: {
    __APP_VERSION__: JSON.stringify(`v${pkg.version}`),
  },
  // ... rest unchanged
});
```

`ProjectLanding.svelte` colophon line:
```svelte
<div class="colophon-meta">jamsesh / Apache-2.0 / {__APP_VERSION__} / 2026</div>
```

`ProjectLanding.test.ts` updated assertion:
```typescript
it('renders the colophon with version and license', () => {
  render(ProjectLanding);
  // Version is sourced from __APP_VERSION__ (Vite define → package.json).
  // Assert semver-ish shape rather than a literal to avoid release-bump rot.
  const colophon = screen.getByText(/jamsesh \/ Apache-2\.0 \/ v?\d+\.\d+\.\d+/i);
  expect(colophon).toBeInTheDocument();
});
```

**Implementation Notes**:
- Vite's `define` replaces the `__APP_VERSION__` identifier at bundle time
  with the quoted string literal — it is **not** `window.__APP_VERSION__`;
  it is a compile-time substitution. TypeScript will flag the unknown global;
  add a declaration in `frontend/src/app.d.ts` (or `vite-env.d.ts`):
  ```typescript
  declare const __APP_VERSION__: string;
  ```
- In the Vitest environment, Vite's `define` block is applied during test
  compilation too, so the test sees the real value from `package.json`
  (currently `"v0.0.1"`). The regex matches any semver-ish string, so no
  test update is needed on future version bumps.
- `frontend/package.json` version (`"0.0.1"`) is the source. The `v` prefix
  is prepended in the define expression. Operators may optionally add a
  `package.json` version update to `scripts/release-bump.sh` to keep
  frontend version in sync with the Go release tag; that is out of scope
  for this story (the story's goal is to eliminate the hardcoded literal,
  not to wire the release pipeline).
- The `with { type: 'json' }` import assertion is supported in Vite/Node
  ≥ 18 and is the correct way to import JSON in ESM. If the project's Node
  version is older, use `createRequire` or `fs.readFileSync` + `JSON.parse`
  as a fallback — but Node 18+ is effectively the baseline for Vite 5.

**Acceptance Criteria**:
- [ ] The colophon renders with a semver-shaped version string (not `v0.4.0`
  hardcoded literal).
- [ ] `ProjectLanding.test.ts` assertion passes on any future `package.json`
  version bump without editing the test.
- [ ] TypeScript compilation does not warn about `__APP_VERSION__` being
  an unknown identifier.
- [ ] `npm run build` completes without error with the `define` block present.

---

## Implementation Order

1. All three stories in parallel — no internal dependencies.

## Testing

### Unit 1: `internal/portal/router/router_test.go`
- Happy path: POST with valid JSON → 204 + log record captured
- Malformed body: POST with `{bad` → 204 (no 400)
- Wrong method: GET → 405 JSON envelope
- No auth header: POST → 204 (unauthenticated route)
- Oversized body: POST with >64 KiB → 204 (MaxBytesReader truncates; log
  parse_err, still 204)

### Unit 2: `internal/portal/portalinfo/handler_test.go`
- Extend `newTestEnv` to apply `NoCacheMiddleware` in the test router
- Add assertion: response header `Cache-Control` == `no-store`
- Existing six test cases still pass

### Unit 3: `frontend/src/lib/screens/ProjectLanding.test.ts`
- Existing colophon test updated to regex match
- All other existing assertions unchanged
- No new test cases needed

## Risks

- **`with { type: 'json' }` import assertion**: supported in Vite 5 + Node
  18+. If the CI environment runs an older Node, the build fails. Mitigation:
  check `.nvmrc` / `engines` field in `package.json` before using this syntax;
  fall back to `fs.readFileSync` approach if needed.
- **Cache-Control header ordering**: Go's `http.ResponseWriter` silently
  drops headers set after `WriteHeader`. The middleware sets the header
  before calling `next.ServeHTTP`, which is correct — but any future
  refactor that moves the middleware call after `next` would silently break
  it. The test on Unit 2 pins this.
- **BodyLimit on `/_csp-report`**: the global BodyLimit only covers
  `/api/*`. The CSP endpoint needs its own `MaxBytesReader` guard to
  prevent large payloads from exhausting memory. Explicitly called out in
  Unit 1 implementation notes.
