---
id: bug-squash-forkdialog-empty-org-refs-fetch
kind: story
stage: implementing
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
bug_location: frontend/src/lib/components/ForkDialog.svelte:48
---

# ForkDialog refs fetch always fails because orgIdFromRef returns "" — fork targets wrong tip

**Location**: `frontend/src/lib/components/ForkDialog.svelte:48` · **Severity**: medium · **Pattern**: swallowed error masking a guaranteed-failing request

`orgIdFromRef` always returns `''`, so the refs request goes to `/api/orgs//sessions/.../refs`, which never matches the org-scoped route and returns non-2xx; the `if (refsRes.ok)` guard then skips silently and `commitSha` stays `null`. The fork proceeds without `target_commit_sha`, so it always points at the wrong/default tip rather than the selected source ref's tip, with no error surfaced. ForkDialog never receives `orgId` as a prop even though the parent shell has it. Fix: pass `orgId` into ForkDialog and use it (or the typed `client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs')`), and treat a failed refs fetch as a surfaced error rather than a silent skip.

```ts
const refsRes = await fetch(`/api/orgs/${encodeURIComponent(orgIdFromRef(sourceRef))}/sessions/.../refs`, ...);
if (refsRes.ok) { ... } // orgIdFromRef always "" -> request 404s -> silently skipped
function orgIdFromRef(_ref: string): string { return ''; }
```
