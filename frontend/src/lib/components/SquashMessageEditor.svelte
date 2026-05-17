<script lang="ts">
  // Squash-mode commit-message editor. Owns no state — pure
  // input/output surface driven by the `message` prop and emitting
  // `onmessagechange(value)` on every keystroke. The parent
  // (FinalizeView) holds the source of truth and debounces the PATCH.
  //
  // Embeds <CoAuthorChipRow/> below the textarea; the row hides
  // itself when `coAuthors` is empty (single-author edge case).
  //
  // Visual contract: matches `.msg-editor` in the locked Option-3
  // mock at .mockups/screens/epic-finalize-flow-portal-ui-curation-view/option-3.html
  import type { components } from '$lib/api/types.gen';
  import CoAuthorChipRow from './CoAuthorChipRow.svelte';

  type CoAuthor = components['schemas']['CoAuthor'];

  let {
    message,
    onmessagechange,
    coAuthors,
  }: {
    message: string;
    onmessagechange: (next: string) => void;
    coAuthors: CoAuthor[];
  } = $props();

  function handleInput(e: Event) {
    const target = e.currentTarget as HTMLTextAreaElement;
    onmessagechange(target.value);
  }
</script>

<div class="msg-editor">
  <div class="label-row">
    <label for="squash-message">Commit message</label>
    <span class="hint">
      Squash mode &middot; pre-filled from session goal + commit subjects &middot; editable
    </span>
  </div>
  <textarea
    id="squash-message"
    value={message}
    oninput={handleInput}
    rows="8"
    aria-label="Squash commit message"
  ></textarea>
  <CoAuthorChipRow authors={coAuthors} />
</div>

<style>
  .msg-editor {
    display: grid;
    gap: 8px;
  }

  .label-row {
    display: flex;
    align-items: baseline;
    gap: 10px;
  }

  .msg-editor label {
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.1em;
    color: var(--color-text-secondary);
  }

  .label-row .hint {
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
  }

  .msg-editor textarea {
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

  .msg-editor textarea:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: -1px;
    border-color: var(--color-accent);
  }
</style>
