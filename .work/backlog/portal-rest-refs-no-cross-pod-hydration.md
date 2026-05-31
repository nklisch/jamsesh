---
id: portal-rest-refs-no-cross-pod-hydration
kind: story
stage: backlog
tags: [portal, infra, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# REST `/refs` has no cross-pod hydration hook

## Problem
`GET /api/orgs/{org}/sessions/{sid}/refs` (`ListSessionRefs`,
`internal/portal/sessions/state.go:165-173`) opens the **pod-local** bare repo
directly with `git.PlainOpen(repoPath)`. If the session's repo is absent on the
pod serving the request (e.g. a pod that has never hydrated this session from
object storage), it returns `200 OK` with an empty ref list — silently, with no
hydration attempt.

By contrast, the git smart-HTTP path hydrates on access: `infoRefs` /
`uploadPack` / `receivePack` call `acquireForGitRequest` →
`LifecycleManager.AcquireForRequest` → `Hydrator.Hydrate`
(`internal/portal/githttp/handler.go:82-90`,
`internal/portal/storage/objectstore/lifecycle.go:201`), which pulls the bare
repo from object storage before serving.

Consequence: in clustered (multi-pod) mode, REST `/refs` is **non-deterministic**
across pods. A client that lands on a pod which has not hydrated the session gets
an empty ref list and a misleading `200`, instead of the true ref set. This is a
latent correctness bug — not currently exercised by the chaos handoff tests
(which read REST `/refs` only after `WaitForHydration` + a git-smart-HTTP push
have already hydrated the target pod), which is why it surfaced during analysis
rather than as a test failure.

## Suggested fix
Route `ListSessionRefs` through the same hydration/lifecycle path the git
smart-HTTP handler uses (acquire-for-request → hydrate) before opening the
pod-local repo, OR read the ref set from the object-storage manifest directly so
the result is pod-independent. Distinguish "repo genuinely has no refs" from
"repo not present on this pod" so the API never reports a false-empty list.

## Origin
Surfaced while root-causing
`e2e-cloud-native-multipod-suite-red-objectstore-sync`. The chaos handoff tests'
immediate red was a separate test-only ref-name mismatch (fixed there); this
hydration gap is the latent product bug noted during that investigation.
</content>
