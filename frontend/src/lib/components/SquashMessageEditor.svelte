<script lang="ts">
  // Placeholder for the squash commit-message editor.
  // Story 1 (FinalizeView + route) lands this as a minimum-viable
  // textarea wired to `message` + `onmessagechange` so the parent's
  // PATCH debounce path is exercised. Story 2
  // (curation-view-squash-editor-and-coauthor-chips) replaces this
  // body with the full editor + collapsed/expanded states + diff
  // chrome.
  import type { components } from '$lib/api/types.gen';
  import CoAuthorChipRow from './CoAuthorChipRow.svelte';

  type CoAuthor = components['schemas']['CoAuthor'];

  let {
    message,
    onmessagechange,
    coAuthors = [],
  }: {
    message: string;
    onmessagechange: (next: string) => void;
    coAuthors?: CoAuthor[];
  } = $props();

  function handleInput(e: Event) {
    const target = e.currentTarget as HTMLTextAreaElement;
    onmessagechange(target.value);
  }
</script>

<div class="squash-editor">
  <label class="label" for="squash-message">Commit message</label>
  <textarea
    id="squash-message"
    class="textarea"
    value={message}
    oninput={handleInput}
    rows="8"
    aria-label="Squash commit message"
  ></textarea>
  <CoAuthorChipRow authors={coAuthors} />
</div>

<style>
  .squash-editor {
    display: grid;
    gap: var(--space-2);
  }

  .label {
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-text-secondary);
  }

  .textarea {
    width: 100%;
    box-sizing: border-box;
    min-height: 150px;
    resize: vertical;
    padding: 10px 12px;
    border-radius: var(--radius-sm);
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-secondary);
    color: var(--color-text-primary);
    font: var(--font-size-sm) / 1.55 var(--font-mono);
  }

  .textarea:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: -1px;
    border-color: var(--color-accent);
  }
</style>
