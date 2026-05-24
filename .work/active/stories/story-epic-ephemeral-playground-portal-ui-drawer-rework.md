---
id: story-epic-ephemeral-playground-portal-ui-drawer-rework
kind: story
stage: done
tags: [ui]
parent: feature-epic-ephemeral-playground-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-23
updated: 2026-05-23
---

# NewSessionDrawer rework (CLI + skill output)

## Scope

Story 4 of the parent feature. Replaces the NewSessionDrawer's
POST-to-create-API logic with a CLI + skill command-output renderer.
The drawer becomes "what to ask your agent" + "what to type yourself"
generator, parallel to the agent-primary mental model locked in for
the wave-1 cli-first-creation feature.

Full design in the parent feature body's "Story 4" section.

## Files delivered

- `frontend/src/lib/components/NewSessionDrawer.svelte` (modify)
- `frontend/src/lib/components/NewSessionDrawer.test.ts` (extend)

## Acceptance criteria

See the parent feature body's "Story 4 acceptance criteria" section.
Summary: form submit renders two copyable command blocks (skill +
raw CLI); copy buttons work; no API call made on submit; form
validation still runs.

## Notes for the implementing agent

- The two command forms are:
  1. `/jamsesh:jam --org X --goal '<text>' --scope '<glob>' --mode <mode> --invite a@x,b@y`
     (skill form — pastes into CC; consumed by the wave-3 plugin-skills
     feature's `/jamsesh:jam` skill body)
  2. `jamsesh new --org X --goal '<text>' --scope '<glob>' --mode <mode> --invite a@x,b@y`
     (raw CLI form — pastes into a terminal)
- Quote the goal/scope/invite values with single-quotes if they
  contain spaces or special characters. Use a small shell-escape
  helper rather than naively concatenating.
- Empty form fields: omit the corresponding flag entirely (don't render
  `--goal ''`). This keeps the rendered command minimal and idiomatic.
- Clipboard copy: use `navigator.clipboard.writeText(...)` with a small
  success toast / state-change indicator on the button ("Copied!").
- The drawer stays in the same place in the UI (triggered from
  SessionList's "New session" button). The change is just what happens
  on submit.
- This story is independent (`depends_on: []`) — doesn't need the
  router refactor or any of the playground-side changes. Can run in
  parallel with Stories 1, 2, 3.

## Implementation notes

### What changed

**`frontend/src/lib/components/NewSessionDrawer.svelte`** — reworked from a session-creation form to a CLI + skill command-output generator.

- Removed `oncreated` prop and all `client.POST(...)` logic.
- Applied the `view-state-union-machine` pattern: `type ViewState = 'form' | 'output'`. The form view collects goal / scope / mode / invitees; on submit it transitions to the output view.
- Added `shellEscape(value)` helper: wraps values containing shell-special chars in single quotes (with embedded `'` escaped as `'\''`); plain alphanumeric/glob/email values pass through unquoted.
- Empty-flag omission: goal, scope, and invitees flags are excluded when their fields are empty.
- Two `<code>` blocks in the output view: skill form (`/jamsesh:jam ...`) and raw CLI form (`jamsesh new ...`), each with a copy button.
- Clipboard copy via `navigator.clipboard.writeText(...)` with a 2-second "Copied!" state reverting to "Copy".
- "Edit form" button returns to `form` view; "Done" calls `onclose`.
- Removed `name` field (not present in the CLI/skill command signature — goal/scope/mode/invitees are the meaningful params).
- Width bumped from 420px to 480px to accommodate longer command strings.

**`frontend/src/lib/components/NewSessionDrawer.test.ts`** — rebuilt.

- Replaced the 9 API-interaction tests with 20 tests covering: command output structure, flag inclusion/omission, shell quoting, clipboard integration, copy button feedback (including fake-timer revert), and form/output navigation.
- Clipboard mock pattern matches `AttachHelpLink.test.ts` (the existing project precedent for this).

**`frontend/src/lib/screens/SessionList.svelte`** — updated to match new drawer contract.

- Removed `oncreated` prop from `<NewSessionDrawer>` usage (prop no longer exists).
- Removed dead state `walkthroughSessionId` and `handleSessionCreated` function.
- Removed the standalone `<SessionAttachWalkthrough>` that was exclusively triggered by `handleSessionCreated`. The `<AttachHelpLink>` component in the page header remains and carries its own walkthrough instance (the "Setup help" flow is unaffected).

**`frontend/src/lib/screens/SessionList.test.ts`** — updated to match new drawer behavior.

- Updated two tests that checked for `"Create session"` button text → now check for `"Generate commands"`.
- Removed two tests (`successful session creation opens the walkthrough...` and `"Open session view →" inside walkthrough navigates...`) that tested the walkthrough-via-`oncreated` path — this behavior no longer exists by design.
- Removed `createSessionViaDrawer` helper (was the only consumer of those tests).

### Deviations from design spec

- The design mentioned keeping the `name` field. After reading the CLI spec (`jamsesh new` flags), `name` is not a flag — the CLI/skill command doesn't include it. Omitted `name` field to keep the rendered commands accurate and idiomatic. The goal field serves the same descriptive purpose.
- The skill command is `/jamsesh:jam` (per the story's notes) rather than `/jamsesh:new` (per the feature body's design decisions section). The story body takes precedence as it's more specific.

### Observation: unrelated Svelte 4 drift

`ModeSwitchDialog.svelte` has a pre-existing Svelte 4 pattern flagged by svelte-check (captures `currentMode` as initial value instead of via closure). Not in scope here — noted for a future cleanup pass.

### Verification

- `npm run check`: 0 errors, 2 pre-existing warnings (unrelated files)
- `npm run test`: 532 passed, 0 failed, 44 test files
- `npm run build`: valid bundle produced (186 modules, no errors)

## Review (2026-05-23)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**:
- Backdrop `<div>` has both an `onclick` and inline `onkeydown` Escape handler while `<svelte:window onkeydown={handleKeyDown}>` already catches Escape — the inline handler is redundant. Leave or trim, not worth a follow-up item.
- `cmd-text` uses `word-break: break-all` plus `white-space: pre-wrap`, which can wrap mid-flag in very narrow viewports (e.g. `--inv\nite`). Acceptable for a 480px drawer but worth noting if width regresses.

**Notes**:
- All 4 acceptance criteria met: command output renders both forms with substituted values; copy buttons work via `navigator.clipboard.writeText` with a 2-second "Copied!" revert; no `client.POST(...)` call on submit (the API client import is gone); form validation (`orgId` presence, scope-glob parsing) still runs before transitioning to the output view.
- Skill name `/jamsesh:jam` is consistent with the sibling `plugin-skills/jam-consolidation` story; the deviation from the parent feature's design-decisions note (`/jamsesh:new`) is the right call and is documented in implementation notes.
- `shellEscape` uses the canonical POSIX single-quote escape (`'\''` for embedded quotes); the safe-char regex `/^[A-Za-z0-9@._,/*-]+$/` correctly leaves emails/globs unquoted. Tested with embedded spaces.
- `view-state-union-machine` pattern applied cleanly with the typedef comment block describing the state graph.
- Removing `SessionAttachWalkthrough` from `SessionList` is safe — still used by `InviteAccept.svelte` and `AttachHelpLink.svelte` (verified via grep). The removed walkthrough tests covered a code path that no longer exists by design.
- `npm run check` clean (0 errors, 2 pre-existing unrelated warnings). `npm run test` for NewSessionDrawer (27 tests) + SessionList (15 tests) all pass.
- Foundation-doc check: `docs/UX.md` § Flow: creating a session already describes CLI-first creation as the primary path; the drawer-rework aligns with the doc rather than invalidating it. The `/jamsesh:new` reference at UX.md:68 is owned by the plugin-skills consolidation story, not this one.
