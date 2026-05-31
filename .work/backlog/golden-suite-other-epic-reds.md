---
id: golden-suite-other-epic-reds
kind: story
stage: backlog
tags: [testing, bug]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-31
updated: 2026-05-31
---

# Golden suite: three non-cloud-native reds (other epics)

## Brief
While draining `e2e-cloud-native-multipod-suite-red`, the full golden-suite sweep
surfaced three failures that are OUTSIDE that epic's scope (cloud-native
multi-pod: objectstore/lease/router/githttp). They belong to other (already
"done") epics and were caught in the systemic never-green e2e breakage. They are
NOT regressions from the cloud-native stabilization work — each fails on a
domain assertion unrelated to the modified clone/push/lease/router paths
(verified: `gitclient.Clone`/`portal.go` fixture changes are semantics-preserving
for these). Filed for separate triage by the owning epics.

## The three reds
1. **`golden/TestFinalizeLockStateMachine`** (finalize flow) —
   `finalize_plan_test.go:350: patchFinalizeLock: status 400 (want 200):
   {"error":"session.invalid_base_sha","message":"base_sha must be a 40-character
   lowercase hex SHA-1"}`. The test sends a malformed/empty `base_sha` to the
   finalize-lock PATCH. Owning area: finalize epic.
2. **`golden/TestForkAndComment`** (fork + MCP comment) —
   `fork_and_comment_test.go:162: user-prompt-submit additionalContext does not
   contain the comment text "Agent B, please review this initial commit."`.
   Owning area: comments / MCP / auto-merger.
3. **`golden/TestCLI_JamPlayground`** (playground CLI) — fails fast (1.3s);
   playground is EXPLICITLY out of this epic's scope (see the epic body — owned
   by the playground epics). Owning area: ephemeral-playground.

## Disposition
Scope into the relevant epics (or a small triage feature) separately. The
cloud-native multi-pod suite work is complete and green independent of these.
