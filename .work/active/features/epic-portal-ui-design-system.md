---
id: epic-portal-ui-design-system
kind: feature
stage: review
tags: [ui]
parent: epic-portal-ui
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal UI — Design System

## Brief

Realizes the locked design system (Quiet Slate palette + Geist typography)
as actual Svelte 5 base components and a token-import pipeline. The mockup
phase already produced `tokens.css` with both light and dark mode and the
explicit `[data-theme]` toggle mechanism; this feature wires that file into
the Vite build and ships the small set of reusable components every other
portal-UI feature consumes.

Components to ship:

- `Button` (primary / ghost / accent variants matching the palette mocks)
- `Input` (text, email)
- `Card` (the surface container used across sessions and the artifact pane)
- `Badge` / `Pill` (mode pill — sync / isolated; conflict pill)
- `AuthorDot` (takes a user id, returns a colored dot using `--author-N`)
- `ThemeToggle` (cycles light / dark / system, sets `[data-theme]` on
  `<html>`, persists choice)
- `InlineCode` (matching the `<code>` styling in the mocks)

Independent of `foundation` because tokens + base components don't need
routing or auth to design and validate. The two features run in parallel
and the consuming features (session-list, session-view-shell, etc.) join
their output downstream.

Does NOT cover: any session-bound or auth-bound UI (those land in
consuming features).

## Epic context

- Parent epic: `epic-portal-ui`
- Position in epic: independent foundation feature — runs in parallel with
  `epic-portal-ui-foundation`. Every consuming feature depends on both.

## Foundation references

- `docs/SPEC.md` — Stack > Portal frontend
- `docs/UX.md` — Portal UI surfaces (component vocabulary)
- `.mockups/design-system/tokens.css` — the source of truth this feature
  realizes in Svelte
- `.mockups/design-system/palette.html` — visual reference for component
  states
- `.mockups/design-system/typography.html` — type scale + weight usage

## Mockups

This feature's "mockup" IS `.mockups/design-system/` — that's its source of
truth. No `/screens` pass needed (the design system is what other features
mock against, not a screen of its own). What this feature ships:

- `tokens.css` import pipeline (Vite-side; tokens become the single source
  of CSS variable values consumed by every component)
- Base Svelte components implementing the visual treatments shown in
  `palette.html`:
  - `Button` (primary = `bg-inverse` + `text-inverse`, ghost = transparent
    + bordered, accent = `accent` + `text-inverse` — see palette.html
    composition section)
  - `Input` (text, email — focus ring uses `accent` color)
  - `Card` (`bg-secondary` + `border` + `radius-md`)
  - `Badge` (e.g., status pills — `new`, `finalizing`, `ended`)
  - `ModePill` (`sync` = accent-muted/accent, `isolated` = warning-muted/warning)
  - `ConflictPill` (danger-muted + danger, used in activity feeds and tree)
  - `AuthorDot` (round 14-24px, takes a user id → returns `--author-N`
    CSS var lookup, optional `online` modifier adds the green pulse)
  - `InlineCode` (matches `<code>` styling from the palette mocks)
- `ThemeToggle` component that cycles light → dark → system, sets
  `data-theme` on `<html>`, persists to localStorage
- Geist + Geist Mono font loading (via the `@import` already in tokens.css —
  components just use `var(--font-sans)` / `var(--font-mono)`)

Downstream features (`session-list`, `session-view-shell`,
`artifact-and-comments`, `ref-actions`) all consume these base components.
Their mocks already use the same CSS variables, so the component contracts
are visible inline in the option HTML files.

## Design decisions

Resolved at feature-design time (autopilot, judgment branch):

- **Component file location**: `frontend/src/lib/components/` (Svelte 5
  convention). Tokens live at `frontend/src/lib/styles/tokens.css`.
- **Tokens.css source**: copy `.mockups/design-system/tokens.css`
  verbatim. The mockup IS the spec; re-running
  `/ux-ui-design:palette` later regenerates the mockup, and this
  story re-copies. No hand-editing of either file out of band.
- **Build/test setup ownership**: this feature copies `tokens.css`
  and writes `.svelte` files. The Vite + Vitest + Svelte preprocessor
  toolchain belongs to `epic-portal-ui-foundation`. The two features
  declare `depends_on: []` against each other — they land
  independently and integrate by both feeding the same `frontend/`
  tree. Components in this feature are syntactically valid Svelte 5
  but uncompiled until foundation provides the toolchain.
- **Svelte version**: Svelte 5 with runes (`$state`, `$props`,
  `$derived`) per `docs/SPEC.md > Portal frontend`. No Svelte 4
  legacy syntax.
- **TypeScript**: every component file uses `<script lang="ts">`
  with explicit `$props()` typed shapes.
- **Snippets vs slots**: Svelte 5 snippets (`{#snippet}` /
  `{@render}`) for content composition; never the deprecated `<slot>`
  syntax. The `Card`'s body content uses a default snippet named
  `children` (Svelte 5 convention).
- **ThemeToggle persistence**: `localStorage.theme = "system" | "light" | "dark"`.
  On mount, the component reads the value and applies
  `data-theme` to `<html>`. On click, it cycles
  `system → light → dark → system`. A one-time global setup
  (in foundation's app entry) reads localStorage before first paint
  to avoid a FOUC flash; documented in this story as a coordination
  note for foundation.
- **Component testing**: Vitest + `@testing-library/svelte` (Svelte
  5 testing path). Tests scoped to component contract (props in →
  rendered output / behavior out). Visual regression is mocked-only
  for v0 — Playwright integration is a later feature.
- **Story decomposition**: single story. The mocks are already
  authoritative; implementation is mechanical translation. No
  meaningful fan-out target. If a future addition (e.g., new
  component variants) substantially expands the surface, that's a
  separate feature.

## Architectural choice

**Tokens.css as the single CSS-variable source of truth; Svelte 5
components consume `var(--*)` exclusively.** No component-local color
literals, no `style="color: ..."` overrides, no hardcoded font
values. Every visual property routes through a token.

Alternatives considered:

- **Tailwind CSS** — heavy; replaces tokens with utility classes
  and decouples the codebase from the mockup convention. The
  mockup-first principle in CLAUDE.md works against this: mocks
  use raw CSS + tokens, so production code matching that surface
  reduces translation overhead.
- **Component-scoped CSS-in-JS** — Svelte 5's `<style>` blocks
  already give us component-scoped CSS with full access to global
  custom properties. No third-party CSS-in-JS needed.

## Implementation Units

### Unit 1: Tokens import

**Files**:
- `frontend/src/lib/styles/tokens.css` (copy from `.mockups/design-system/tokens.css`)
- `frontend/src/app.css` (one-line import: `@import './lib/styles/tokens.css';`)

**Story**: `epic-portal-ui-design-system-tokens-and-components`

### Unit 2: Button

**File**: `frontend/src/lib/components/Button.svelte`

```svelte
<script lang="ts">
  type Variant = 'primary' | 'ghost' | 'accent';
  type Size = 'sm' | 'md' | 'lg';

  let { variant = 'primary', size = 'md', disabled = false, type = 'button',
        onclick, children }: {
    variant?: Variant;
    size?: Size;
    disabled?: boolean;
    type?: 'button' | 'submit' | 'reset';
    onclick?: (e: MouseEvent) => void;
    children: import('svelte').Snippet;
  } = $props();
</script>

<button class="btn btn-{variant} btn-{size}" {type} {disabled} {onclick}>
  {@render children()}
</button>

<style>
  .btn {
    font-family: var(--font-sans);
    font-weight: var(--font-weight-medium);
    border-radius: var(--radius-md);
    cursor: pointer;
    border: 1px solid transparent;
    transition: background-color 120ms, color 120ms, border-color 120ms;
  }
  .btn:disabled { opacity: 0.5; cursor: not-allowed; }
  .btn-sm { font-size: var(--font-size-sm); padding: var(--space-1) var(--space-3); }
  .btn-md { font-size: var(--font-size-base); padding: var(--space-2) var(--space-4); }
  .btn-lg { font-size: var(--font-size-lg); padding: var(--space-3) var(--space-6); }
  .btn-primary { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .btn-primary:hover:not(:disabled) { background: var(--color-text-secondary); }
  .btn-ghost { background: transparent; color: var(--color-text-primary); border-color: var(--color-border); }
  .btn-ghost:hover:not(:disabled) { background: var(--color-bg-tertiary); }
  .btn-accent { background: var(--color-accent); color: var(--color-text-inverse); }
  .btn-accent:hover:not(:disabled) { background: var(--color-accent-hover); }
</style>
```

### Unit 3: Input

**File**: `frontend/src/lib/components/Input.svelte`

```svelte
<script lang="ts">
  let { value = $bindable(''), type = 'text', placeholder = '',
        disabled = false, oninput }: {
    value?: string;
    type?: 'text' | 'email';
    placeholder?: string;
    disabled?: boolean;
    oninput?: (e: Event) => void;
  } = $props();
</script>

<input class="input" {type} bind:value {placeholder} {disabled} {oninput} />

<style>
  .input {
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    color: var(--color-text-primary);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    width: 100%;
  }
  .input:focus { outline: 2px solid var(--color-accent); outline-offset: -1px; }
  .input:disabled { opacity: 0.5; cursor: not-allowed; }
</style>
```

### Unit 4: Card

**File**: `frontend/src/lib/components/Card.svelte`

```svelte
<script lang="ts">
  let { padding = 'md', children }: {
    padding?: 'sm' | 'md' | 'lg';
    children: import('svelte').Snippet;
  } = $props();
</script>

<div class="card card-p-{padding}">{@render children()}</div>

<style>
  .card {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
  }
  .card-p-sm { padding: var(--space-3); }
  .card-p-md { padding: var(--space-4); }
  .card-p-lg { padding: var(--space-6); }
</style>
```

### Unit 5: Badge / ModePill / ConflictPill

**File**: `frontend/src/lib/components/Badge.svelte`

A single primitive with variant prop covers all three. Variants:
`neutral | success | warning | danger | accent | sync | isolated | conflict`.

```svelte
<script lang="ts">
  type Variant = 'neutral' | 'success' | 'warning' | 'danger' | 'accent' | 'sync' | 'isolated' | 'conflict';
  let { variant = 'neutral', children }: { variant?: Variant; children: import('svelte').Snippet } = $props();
</script>

<span class="pill pill-{variant}">{@render children()}</span>

<style>
  .pill {
    display: inline-flex;
    align-items: center;
    gap: var(--space-1);
    padding: 2px var(--space-2);
    border-radius: var(--radius-full);
    font-family: var(--font-mono);
    font-size: var(--font-size-xs);
    font-weight: var(--font-weight-medium);
  }
  .pill-neutral { background: var(--color-bg-tertiary); color: var(--color-text-secondary); }
  .pill-success, .pill-sync { background: var(--color-accent-muted); color: var(--color-accent); }
  .pill-warning, .pill-isolated { background: var(--color-warning-muted); color: var(--color-warning); }
  .pill-danger, .pill-conflict { background: var(--color-danger-muted); color: var(--color-danger); }
  .pill-accent { background: var(--color-accent); color: var(--color-text-inverse); }
</style>
```

`ModePill` and `ConflictPill` are then thin wrappers over `Badge`:

```svelte
<!-- ModePill.svelte -->
<script lang="ts">
  import Badge from './Badge.svelte';
  let { mode }: { mode: 'sync' | 'isolated' } = $props();
</script>
<Badge variant={mode}>{mode}</Badge>
```

```svelte
<!-- ConflictPill.svelte -->
<script lang="ts">
  import Badge from './Badge.svelte';
</script>
<Badge variant="conflict">conflict</Badge>
```

### Unit 6: AuthorDot

**File**: `frontend/src/lib/components/AuthorDot.svelte`

```svelte
<script lang="ts">
  // Stable hash of authorId to 1..8 for --author-N variable lookup.
  function authorColorIndex(id: string): number {
    let h = 0;
    for (let i = 0; i < id.length; i++) {
      h = ((h << 5) - h) + id.charCodeAt(i);
      h |= 0;
    }
    return (Math.abs(h) % 8) + 1;
  }

  let { authorId, size = 16, online = false, title }: {
    authorId: string;
    size?: number;
    online?: boolean;
    title?: string;
  } = $props();

  const idx = $derived(authorColorIndex(authorId));
</script>

<span class="dot {online ? 'online' : ''}"
      style="--dot-color: var(--author-{idx}); --dot-size: {size}px"
      title={title ?? authorId}
      role="img"
      aria-label={title ?? `author ${authorId}`}>
  {#if online}<span class="pulse"></span>{/if}
</span>

<style>
  .dot {
    position: relative;
    display: inline-block;
    width: var(--dot-size);
    height: var(--dot-size);
    border-radius: var(--radius-full);
    background: var(--dot-color);
    flex-shrink: 0;
  }
  .pulse {
    position: absolute;
    bottom: -2px;
    right: -2px;
    width: 33%;
    height: 33%;
    min-width: 6px;
    min-height: 6px;
    background: var(--color-success);
    border: 2px solid var(--color-bg-secondary);
    border-radius: var(--radius-full);
  }
</style>
```

### Unit 7: InlineCode

**File**: `frontend/src/lib/components/InlineCode.svelte`

```svelte
<script lang="ts">
  let { children }: { children: import('svelte').Snippet } = $props();
</script>

<code class="inline-code">{@render children()}</code>

<style>
  .inline-code {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    background: var(--color-bg-tertiary);
    color: var(--color-text-primary);
    padding: 1px var(--space-1);
    border-radius: var(--radius-sm);
  }
</style>
```

### Unit 8: ThemeToggle

**File**: `frontend/src/lib/components/ThemeToggle.svelte`

```svelte
<script lang="ts">
  import { onMount } from 'svelte';

  type Theme = 'system' | 'light' | 'dark';
  const KEY = 'jamsesh.theme';

  let theme = $state<Theme>('system');

  onMount(() => {
    const saved = localStorage.getItem(KEY) as Theme | null;
    if (saved === 'light' || saved === 'dark' || saved === 'system') {
      theme = saved;
      apply(saved);
    }
  });

  function apply(t: Theme) {
    const root = document.documentElement;
    if (t === 'system') root.removeAttribute('data-theme');
    else root.setAttribute('data-theme', t);
  }

  function cycle() {
    theme = theme === 'system' ? 'light' : theme === 'light' ? 'dark' : 'system';
    localStorage.setItem(KEY, theme);
    apply(theme);
  }
</script>

<button class="theme-toggle" onclick={cycle} aria-label="theme: {theme}" title="theme: {theme}">
  {#if theme === 'system'}
    <span aria-hidden="true">⏾</span>
  {:else if theme === 'light'}
    <span aria-hidden="true">☀</span>
  {:else}
    <span aria-hidden="true">●</span>
  {/if}
</button>

<style>
  .theme-toggle {
    background: transparent;
    border: 1px solid var(--color-border);
    color: var(--color-text-secondary);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-md);
    cursor: pointer;
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
  }
  .theme-toggle:hover { background: var(--color-bg-tertiary); }
</style>
```

### Unit 9: FOUC pre-paint helper (coordination note)

Add to `frontend/src/lib/styles/theme-bootstrap.ts`:

```ts
// Pre-paint helper. Read by foundation's app entry as the very first
// import to set data-theme BEFORE Svelte hydration paints. Avoids
// the FOUC where the system theme paints then immediately swaps.
const saved = typeof localStorage !== 'undefined'
  ? (localStorage.getItem('jamsesh.theme') as 'system' | 'light' | 'dark' | null)
  : null;
if (saved === 'light' || saved === 'dark') {
  document.documentElement.setAttribute('data-theme', saved);
}
```

Foundation's `app.ts` will import this synchronously before any
Svelte mount.

## Implementation Order

Single story (`tokens-and-components`):

1. Copy `tokens.css` into `frontend/src/lib/styles/`
2. Author component files in `frontend/src/lib/components/`
3. Add `theme-bootstrap.ts` for foundation to consume
4. Author component test files in `frontend/src/lib/components/*.test.ts`
   (will only run once foundation lands Vitest)

## Testing

Tests use `@testing-library/svelte` + Vitest. Each component gets one
`*.test.ts` file co-located with the component covering:
- Default render shape
- Each variant / size prop combination produces the expected class
- Click handlers fire when triggered
- Disabled state prevents interaction (Button, Input)
- `bind:value` round-trips (Input)
- AuthorDot hash function is stable (same ID → same color index)
- ThemeToggle cycles state correctly (mocked localStorage)

Tests are authored in this story but only run once foundation lands
the Vitest config. Document this in the story body.

## Risks

- **Svelte 5 syntax churn.** Snippets / runes are stable as of
  Svelte 5.0; pin `svelte@^5.0.0` in package.json. The
  `svelte5-best-practices` skill (auto-loaded) carries the canonical
  patterns.
- **Theme-toggle FOUC.** Documented in Unit 9 via the bootstrap
  helper. The risk is operationalized as a coordination note for
  foundation; if foundation skips the helper, the theme flashes on
  page load. Test this end-to-end once foundation lands.
- **Test-environment dependency on foundation.** Component tests
  are inert until Vitest configuration lands in foundation. Mitigation:
  the test files are still valuable as executable spec — manual
  inspection during code review can catch obvious bugs even before
  the runner is wired up.

## Implementation summary

1 child story (tokens-and-components) at review. 21 component-test failures flagged for follow-up.

### Verification
- `go build ./...` clean
- `go test ./...` green (Go side)
- `go vet ./...` clean
