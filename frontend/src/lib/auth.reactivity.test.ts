// auth.reactivity.test.ts
//
// Tests subscriber-observable reactivity for auth.addOrg — specifically that
// a Svelte $effect which reads auth.orgs re-runs after auth.addOrg() is
// called. This is a distinct property from the reference-inequality already
// covered in auth.test.ts ("addOrg appends to existing orgs array via
// reassignment"): a reassignment that bypassed Svelte's signal tracking would
// also produce a new reference but would NOT re-fire consumer effects.
//
// This file intentionally does NOT call vi.resetModules() between tests.
// vi.resetModules() evicts the `svelte` package from the module cache along
// with every other module. A subsequently re-imported auth.svelte.ts therefore
// gets a different Svelte runtime instance than the AuthOrgsReactivityHarness
// component (which was evaluated before the reset), so the auth module's
// $state signals live in a different scheduler context and cross-module reactive
// tracking breaks silently. By keeping both auth and the harness in the same
// module graph (no reset), they share one runtime and reactivity works as
// intended.

import { describe, it, expect, afterEach } from 'vitest';
import { render, screen, waitFor, cleanup } from '@testing-library/svelte';
import AuthOrgsReactivityHarness from './AuthOrgsReactivityHarness.test.svelte';

// Mock router.svelte before auth.svelte is imported. auth.svelte calls
// `import { navigate } from '$lib/router.svelte'` at module scope, so the
// mock must be declared before the first import that transitively loads auth.
// vi.mock() is hoisted above all imports by Vitest's transform, so this
// declaration — wherever it physically appears in the file — runs first.
vi.mock('$lib/router.svelte', () => ({
  navigate: vi.fn(),
  current: { name: 'sessions', params: {} },
}));

// Import auth after vi.mock so it binds to the mocked router.
// Both this import and the harness component's `import { auth } from './auth.svelte'`
// resolve to the same module instance (same Svelte runtime, same signals).
import { auth } from '$lib/auth.svelte';

describe('auth.addOrg — subscriber-observable reactivity', () => {
  afterEach(() => {
    cleanup();
  });

  it('addOrg triggers $effect re-run for orgs subscribers', async () => {
    // Mount the harness component. It reads auth.orgs inside a $effect,
    // which registers _orgs as a reactive dependency. The initial run of
    // the effect (component mount) sets effectRunCount to 1.
    const { unmount } = render(AuthOrgsReactivityHarness);

    await waitFor(() => {
      expect(screen.getByTestId('effect-run-count').textContent).toBe('1');
    });

    // Call addOrg — this reassigns _orgs via `_orgs = [..._orgs, org]`,
    // signalling the Svelte reactive runtime that the value changed.
    // Subscribers (i.e. the harness $effect) must be scheduled to re-run.
    auth.addOrg({ id: 'rx-org', name: 'Reactive Org', slug: 'rx-org', role: 'member' });

    // The effect re-run increments effectRunCount to ≥ 2 once scheduled.
    // waitFor polls until the DOM reflects the updated value.
    await waitFor(() => {
      const count = parseInt(
        screen.getByTestId('effect-run-count').textContent ?? '0',
        10,
      );
      expect(count).toBeGreaterThanOrEqual(2);
    });

    unmount();
  });
});
