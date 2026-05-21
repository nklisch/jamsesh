---
id: story-portal-session-attach-onboarding-walkthrough-component
kind: story
stage: implementing
tags: [ui]
parent: feature-portal-session-attach-onboarding
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-20
updated: 2026-05-20
---

# SessionAttachWalkthrough shared component

Foundation story for the attach-onboarding feature. Implements the
shared modal component at
`frontend/src/lib/components/SessionAttachWalkthrough.svelte`.

The visual reference is locked at
`.mockups/screens/portal-session-attach-onboarding/option-6.html` —
port the DOM and CSS structure faithfully (same class names, same
overall layout). Drop the mock's `.mock-meta` state toggle (dev-only).

The full design is in the parent feature body under
`## Implementation Units → Unit 1`. Read that first — it contains the
props contract, internal-state design, behavior spec, accessibility
requirements, and the full acceptance-criteria checklist.

## Brief acceptance criteria summary

(See parent feature for the full list.)

- Component contract: `{ open, sessionId, onclose, onopenSession? }` props
- localStorage key `jamsesh.attach-walkthrough-dismissed` controls
  full-vs-compact mode on mount
- "First-time setup?" link in compact mode toggles to full for the
  current modal lifetime only
- Click-to-copy via `navigator.clipboard.writeText`; "Copied" badge for ~1.2s
- ESC / backdrop click / Close button all call `onclose`
- "Open session view →" calls `onopenSession ?? onclose`
- When `sessionId === null` (chrome-help mode): CC pane shows a placeholder
  hint instead of a copyable join line; install commands still copyable
- Long commands ellipsis-truncate visually; full string is what gets copied
- Real Claude Code icon embedded inline (path from
  `/home/nathan/Downloads/claudecode-color.svg`, `#D97757` fill)

## Test file

`frontend/src/lib/components/SessionAttachWalkthrough.test.ts` (new).

Setup notes:
- jsdom needs `navigator.clipboard` mocked explicitly:
  `Object.defineProperty(globalThis.navigator, 'clipboard', { value: {
  writeText: vi.fn().mockResolvedValue(undefined) }, configurable: true });`
- `localStorage.clear()` in `beforeEach`; set the dismissed flag
  explicitly in tests that need compact mode.

## Negative-case discipline

Before claiming completion, mutate the SUT (e.g. remove the localStorage
read on mount) and confirm the relevant test fails. Restore. Document in
implementation notes.
