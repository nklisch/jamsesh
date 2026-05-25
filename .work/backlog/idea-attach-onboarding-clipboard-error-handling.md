---
id: idea-attach-onboarding-clipboard-error-handling
created: 2026-05-21
tags: [ui, bug]
---

`SessionAttachWalkthrough.svelte:100-106` calls `await
navigator.clipboard.writeText(cmd)` with no rejection handling. If the
browser denies clipboard access (mixed-content HTTP page, non-secure
context, permissions-policy block, extension sandbox), the promise
rejects and the unhandled throw propagates out of the click handler.
The "Copied" feedback never fires and the user has no idea the copy
silently failed.

Found in the v0.3.1 review of `feature-portal-session-attach-onboarding`.

Fix shape: wrap in try/catch; on rejection, show a brief "Copy failed —
select and copy manually" hint instead of the "Copied" badge. Same
treatment in `tests/wrapper/install.bats`-style negative path: add a
test that mocks `clipboard.writeText` to reject and asserts the
graceful failure UI.
