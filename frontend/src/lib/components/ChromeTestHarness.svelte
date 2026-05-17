<script lang="ts">
  import Chrome from './Chrome.svelte';

  let { orgChip = '', sessionChip = '' }: { orgChip?: string; sessionChip?: string } = $props();
</script>

<Chrome {orgChip} {sessionChip}>
  {#snippet children()}
    <span data-testid="chrome-body">body content</span>
  {/snippet}
</Chrome>
