---
id: bug-squash-magic-link-fetch-no-trycatch
kind: story
stage: review
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-async-races
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: medium
bug_domain: async
bug_location: frontend/src/lib/screens/Login.svelte:110
---

# requestMagicLink raw fetch has no try/catch — network failure silently hangs the form

**Location**: `frontend/src/lib/screens/Login.svelte:110` · **Severity**: medium · **Pattern**: unhandled promise rejection / swallowed error

`fetch` rejects (rather than returning a non-ok Response) on transport failures — offline, DNS, CORS, aborted connection. With no `try/catch`, a network failure makes the `await` throw, the handler's promise reject unhandled, and neither the `magic-link-error` nor `magic-link-sent` branch runs: the UI stays in `choose` with no error shown and the rejection is lost. The sibling `signInWithGitHub` in the same file already wraps its call in try/catch, so this is an inconsistency. Fix: wrap the await in try/catch and set `mode = 'magic-link-error'` + `errorMsg` in the catch.

```ts
const res = await fetch('/api/auth/magic-link/request', { method: 'POST', ... });
if (res.ok) { mode = 'magic-link-sent'; } else { mode = 'magic-link-error'; ... }
// no catch: transport failure -> unhandled rejection, UI stuck
```

## Implementation notes

Wrapped the `requestMagicLink` fetch in a try/catch block, matching the
`signInWithGitHub` sibling. A transport failure (network error, DNS, offline,
aborted connection) now sets `mode = 'magic-link-error'` and `errorMsg` instead
of leaving an unhandled rejection and hanging the form in the `choose` state.

A regression test was added: mock `fetch` to reject with `TypeError('Failed to fetch')`,
submit the form, and assert the UI shows the error mode with the error message.
