<script lang="ts">
  // Test-only harness component for createTreeState seam-contract verification.
  //
  // Provides a minimal Svelte component context so createTreeState's internal
  // $effect (for localStorage persistence) can run without triggering
  // effect_orphan errors. Exposes the current value in the DOM and a "cycle"
  // button so tests can observe state transitions via @testing-library/svelte.

  import { createTreeState } from './useTreeState.svelte';

  let { sessionId }: { sessionId: string } = $props();

  // Wrap in a closure so svelte-check knows sessionId is accessed reactively.
  const ts = createTreeState((() => sessionId)());
</script>

<span data-testid="tree-state-value">{ts.value}</span>
<button data-testid="cycle-btn" onclick={ts.cycle}>cycle</button>
