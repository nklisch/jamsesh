<script lang="ts">
  // Placeholder. Story 1 just defines the prop interface and renders
  // nothing. Story 2 implements the AuthorDot-decorated chip row.
  import type { components } from '$lib/api/types.gen';

  type CoAuthor = components['schemas']['CoAuthor'];

  let {
    // eslint-disable-next-line @typescript-eslint/no-unused-vars
    authors = [],
  }: {
    authors?: CoAuthor[];
  } = $props();
</script>

<!--
  Intentionally empty. Story 2 fills in the chip row implementation
  (AuthorDot per author + name chip). FinalizeView still passes the
  prop so swapping in the real implementation is a one-line lift.
-->
