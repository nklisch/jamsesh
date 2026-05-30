---
id: bug-squash-artifactpane-stale-fetch-overwrite
kind: story
stage: drafting
tags: [bug, ui, async, high]
parent: epic-bug-squash-frontend-async-races
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-30
updated: 2026-05-30
bug_origin: scan
bug_severity: high
bug_domain: async
bug_location: frontend/src/lib/components/ArtifactPane.svelte:25
---

# ArtifactPane $effect fetch has no stale-response guard — wrong file's content can render

**Location**: `frontend/src/lib/components/ArtifactPane.svelte:25` · **Severity**: high · **Pattern**: race in user-input handler (stale-overwriting-fresh)

The `$effect` re-runs and fires a new `fetch` whenever `selectedSha`/`selectedPath` change, with no cancellation or request-sequence guard. If the user clicks file A then quickly B and A's response arrives after B's, A's body is written into `content`/`mime`/`isBinary`, so the pane shows the wrong file for the current selection (and writes state after the effect run is torn down). The sibling `useFinalizePlan._flushPatch` already uses a `seq` guard idiom; this effect lacks it. Fix: capture an `AbortController` or monotonic request id in the effect, abort in the cleanup return, and short-circuit on resolve when superseded.

```ts
$effect(() => {
  if (!selectedSha || !selectedPath) { /* reset */ return; }
  loading = true;
  fetch(url, { headers }).then((r) => ...).then((data) => { content = data.content; ... });
});
```
