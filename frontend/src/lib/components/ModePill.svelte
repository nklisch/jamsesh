<script lang="ts">
  import Badge from './Badge.svelte';

  let {
    mode,
  }: {
    mode: 'sync' | 'isolated';
  } = $props();
</script>

<Badge variant={mode}>{mode}</Badge>
