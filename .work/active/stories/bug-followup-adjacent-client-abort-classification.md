---
id: bug-followup-adjacent-client-abort-classification
created: 2026-05-30
tags: [bug, portal, error-handling]
bug_origin: gate
bug_severity: low
bug_domain: error-handling
bug_location: internal/portal/auth
---

# Adjacent client-abort / body-read error misclassifications

Surfaced by the codex final-gate review of `epic-bug-squash` (out of scope —
adjacent to the `git-auth-client-abort-500` fix). Two more handler
error-classification gaps of the same family that `epic-bug-squash-handler-error-classification`
did NOT cover: (1) bearer-auth token validation that is cancelled by client
disconnect can be reported as a dependency 503 rather than a client-abort; and
(2) git receive-pack request-body read errors can surface as a 413 when they are
really client aborts. Apply the same `r.Context().Err()`-based client-abort
discrimination (499 / no-5xx) used in `bug-squash-git-auth-client-abort-500`.
Low severity (alert-hygiene), but completes the client-abort classification
sweep.
