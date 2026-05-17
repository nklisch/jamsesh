---
id: epic-portal-ui-design-system-fix-component-tests
kind: story
stage: implementing
tags: [ui, bug]
parent: epic-portal-ui-design-system
depends_on: [epic-portal-ui-foundation-vite-svelte-routing]
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Design System — Fix Component Tests (Snippet API)

## Scope

Follow-up story filed during review of
`epic-portal-ui-design-system-tokens-and-components`. The
component implementations are correct; the test files use an
incorrect Svelte 5 Snippet pattern (`children: () => 'string'`)
which compiles to a TypeScript error and produces 21 failing
tests under `vitest run`.

Affected test files:
- `frontend/src/lib/components/Badge.test.ts`
- `frontend/src/lib/components/Button.test.ts`
- `frontend/src/lib/components/Card.test.ts`
- `frontend/src/lib/components/InlineCode.test.ts`

## Correct pattern

Two options work:

**Option A — `createRawSnippet`:**

```ts
import { createRawSnippet } from 'svelte';

const childrenSnippet = createRawSnippet(() => ({
  render: () => '<span>body</span>',
}));

render(Button, { props: { children: childrenSnippet, variant: 'primary' } });
```

**Option B — test harness companion** (cleaner for complex cases):

```svelte
<!-- ButtonTestHarness.svelte -->
<script lang="ts">
  import Button from './Button.svelte';
  let { variant = 'primary' } = $props();
</script>
<Button {variant}>
  {#snippet children()}<span data-testid="body">test body</span>{/snippet}
</Button>
```

```ts
render(ButtonTestHarness, { props: { variant: 'primary' } });
```

The login-and-chrome story used Option B for Chrome tests
(`ChromeTestHarness.svelte`) — that pattern is the reference.

## Acceptance Criteria

- [ ] `cd frontend && npx svelte-check` is clean (0 errors)
- [ ] `cd frontend && npm run test` is green (21 previously-failing tests now pass)
- [ ] No production-code changes to `Badge.svelte`, `Button.svelte`,
      `Card.svelte`, `InlineCode.svelte` — the bug is purely in
      test files
- [ ] The chosen pattern (A or B) is consistent across all four
      test files

## Notes

- The dep on `epic-portal-ui-foundation-vite-svelte-routing` is
  for the Vitest config (without it, the tests are inert anyway).
  That story already shipped.
- This story is small (~30 min of work) — single agent.
