<script lang="ts">
  // Read-only pill row of contributors for the squash commit. One chip
  // per CoAuthor — AuthorDot marker (8px) + monospaced display name.
  //
  // Chip color seed: `account_id ?? email`. This mirrors how
  // FinalizeView seeds the cart-item AuthorDot for PlanCommits
  // (`account_id ?? author_email`), so the same contributor renders the
  // same color whether they appear in the cart row or as a chip.
  //
  // Renders nothing when `authors` is empty — no label, no wrapper —
  // so the parent's grid gap doesn't open up a vacant slot in
  // single-commit / single-author edge cases.
  import type { components } from '$lib/api/types.gen';
  import AuthorDot from './AuthorDot.svelte';

  type CoAuthor = components['schemas']['CoAuthor'];

  let { authors }: { authors: CoAuthor[] } = $props();

  function colorSeed(a: CoAuthor): string {
    return a.account_id ?? a.email;
  }
</script>

{#if authors.length > 0}
  <div class="coauthors" data-testid="coauthor-chip-row">
    <span class="label">Co-authors</span>
    {#each authors as author (colorSeed(author))}
      <span class="chip" data-testid="coauthor-chip">
        <AuthorDot authorId={colorSeed(author)} size={8} title={author.name} />
        <span class="name">{author.name}</span>
      </span>
    {/each}
  </div>
{/if}

<style>
  .coauthors {
    display: flex;
    flex-wrap: wrap;
    gap: 6px;
    padding: 4px 0;
  }

  .label {
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-text-tertiary);
    align-self: center;
    margin-right: 4px;
  }

  .chip {
    display: inline-flex;
    gap: 6px;
    align-items: center;
    padding: 3px 9px;
    border-radius: var(--radius-full);
    background: var(--color-bg-tertiary);
    border: 1px solid var(--color-border);
    font: var(--font-size-xs) var(--font-mono);
    color: var(--color-text-secondary);
  }

  .name {
    font: var(--font-size-xs) var(--font-mono);
  }
</style>
