---
id: bug-squash-forkdialog-empty-org-refs-fetch
kind: story
stage: done
tags: [bug, ui, async]
parent: epic-bug-squash-frontend-async-races
depends_on: []
release_binding: v0.5.0
gate_origin: null
created: 2026-05-30
updated: 2026-05-31
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

## Implementation notes

Added an `orgId` prop to `ForkDialog` and threaded it from `SessionViewShell`
(which already has `orgId` in scope). Replaced the raw `fetch` with the typed
`client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs')`, using the real
`orgId` in the path params.

Deleted the dead `orgIdFromRef` helper.

Per the codex must-fix: a refs-fetch failure OR an unresolved source ref now
surfaces an error and stops before issuing the fork MCP call (no more silent
`null`-sha fork). The fork only proceeds when the source ref is found in the
refs response, and `target_commit_sha` is always provided.

Four tests added: refs URL uses real org id; fetch failure → error + no fork;
unresolved ref → error + no fork; resolved ref → fork issued with `target_commit_sha`.
