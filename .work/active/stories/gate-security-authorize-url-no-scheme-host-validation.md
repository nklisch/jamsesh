---
id: gate-security-authorize-url-no-scheme-host-validation
kind: story
stage: review
tags: [security]
parent: null
depends_on: []
release_binding: v0.3.0
gate_origin: security
created: 2026-05-20
updated: 2026-05-20
---

# authorize_url from backend redirected without scheme/host validation

## Severity
Low

## Domain
Input Validation & Injection

## Location
`frontend/src/lib/screens/Login.svelte:72,78`

## Evidence
```ts
if (!error && data) authorizeUrl = data.authorize_url;
...
if (authorizeUrl) {
  window.location.assign(authorizeUrl);
```

## Remediation direction
The string from the backend is fed straight into
`window.location.assign()`. If the backend is ever misconfigured (e.g.
operator misenters an OAuth provider, configures a malicious provider,
or a future provider supports `javascript:` URIs for "deep-link" auth)
the SPA will obediently navigate the user there.

Defense in depth: parse with `new URL(authorizeUrl)` and assert the
scheme is `https:` (or `http:` only when origin matches a dev allowlist)
and that the hostname is on a small known-provider allowlist
(`github.com`, `accounts.google.com`, etc.) before assigning. Surface a
sign-in error otherwise.

## Implementation notes

### What was validated / changed

- Added `AUTHORIZE_HOST_ALLOWLIST = ['github.com'] as const` as a `const`
  in the `<script>` block of `Login.svelte`, directly before `signInWithGitHub`.
- Inside `signInWithGitHub`, inside the `if (authorizeUrl)` branch, added:
  1. `try { parsed = new URL(authorizeUrl) } catch { ... }` — rejects malformed URLs.
  2. Protocol + hostname guard: rejects anything that isn't `https:` and
     a hostname on `AUTHORIZE_HOST_ALLOWLIST`. On failure: sets
     `mode = 'oauth-error'`, `errorMsg = 'Authorization URL could not be
     validated. Please try again.'`, `oauthPending = false`, then returns.
  3. `window.location.assign(authorizeUrl)` only reached after both checks pass.
- Decision: no `http:` localhost dev allowance — `provider: 'github'` is
  hardcoded; GitHub OAuth always runs over HTTPS in all environments.

### Allowlist contents
`['github.com']`

### Tests added (Login.test.ts)
- `rejects a non-https authorize_url and shows the error UI`
  → mocks `http://github.com/...`, asserts assign NOT called, error UI shown.
- `rejects a javascript: authorize_url and shows the error UI`
  → mocks `javascript:alert(1)`, asserts assign NOT called, error UI shown.
- `rejects an off-allowlist host authorize_url and shows the error UI`
  → mocks `https://evil.com/authorize`, asserts assign NOT called, error UI shown.
- `rejects a malformed authorize_url and shows the error UI`
  → mocks `'not a url'`, asserts assign NOT called, error UI shown.
- Existing happy-path test (`OAuth button posts to /api/auth/oauth/start and
  assigns the returned authorize_url`) continues to pass with
  `https://github.com/login/oauth/authorize?state=abc`.

### Negative-case verification
Temporarily removed the validation block (reverted to unconditional
`window.location.assign(authorizeUrl)`). All 4 new security tests failed as
expected — `assignSpy` was called and the error UI was absent. Restored the
validation; all 472 tests pass on two consecutive runs.

### Design-flaw check
No GitHub Enterprise or other hostname variant is wired in the codebase
(`provider: 'github'` is hardcoded). Simple `github.com` allowlist is safe.
