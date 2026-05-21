<script lang="ts">
  // Test-only harness component for auth.addOrg reactivity verification.
  //
  // Imports auth directly (not via prop) so it shares the same Svelte runtime
  // instance and signal registry as the test. If auth were injected as a prop
  // and the test called vi.resetModules() before importing auth, the fresh
  // auth module would bind to a different Svelte runtime instance than the
  // component (whose module was evaluated before the reset), breaking
  // cross-module reactive tracking.
  //
  // Reads auth.orgs inside a $effect so Svelte's tracker registers _orgs as
  // a dependency. The initial effect run counts as 1; each subsequent re-run
  // after an auth.addOrg() call increments effectRunCount so tests can assert
  // count ≥ 2 to confirm subscriber-observable reactivity.
  //
  // untrack() is used for the read inside effectRunCount++ so the counter
  // does not subscribe the effect to itself (effectRunCount++ reads the
  // current value, creating a read-then-write cycle that would produce an
  // infinite update loop without untrack).

  import { untrack } from 'svelte';
  import { auth } from './auth.svelte';

  let effectRunCount = $state(0);

  $effect(() => {
    // Reading auth.orgs registers _orgs (a module-level $state in auth.svelte.ts)
    // as a reactive dependency of this effect. Any reassignment of _orgs —
    // including the spread-reassign in addOrg — will schedule a re-run.
    auth.orgs;
    // Use untrack to read the current counter value without subscribing to it.
    effectRunCount = untrack(() => effectRunCount) + 1;
  });
</script>

<span data-testid="effect-run-count">{effectRunCount}</span>
