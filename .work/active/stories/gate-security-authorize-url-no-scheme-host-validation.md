---
id: gate-security-authorize-url-no-scheme-host-validation
kind: story
stage: implementing
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
