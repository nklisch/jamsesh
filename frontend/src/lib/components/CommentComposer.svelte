<script lang="ts">
  import { client } from '$lib/api/client';
  import type { components } from '$lib/api/types.gen';
  import Button from './Button.svelte';

  type Comment = components['schemas']['Comment'];
  type CommentKind = components['schemas']['CommentKind'];

  let {
    orgId,
    sessionId,
    anchorCommitSha,
    anchorFilePath = null,
    anchorLineStart = null,
    anchorLineEnd = null,
    onsubmit,
    oncancel,
  }: {
    orgId: string;
    sessionId: string;
    anchorCommitSha: string;
    anchorFilePath?: string | null;
    anchorLineStart?: number | null;
    anchorLineEnd?: number | null;
    onsubmit?: (comment: Comment) => void;
    oncancel?: () => void;
  } = $props();

  let body = $state('');
  let kind = $state<CommentKind>('question');
  let addressedTo = $state('');
  let submitting = $state(false);
  let submitError = $state<string | null>(null);

  const KINDS: CommentKind[] = ['question', 'suggestion', 'action-request', 'fyi'];

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!body.trim()) return;
    submitting = true;
    submitError = null;

    const reqBody: components['schemas']['CreateCommentRequest'] = {
      anchor_commit_sha: anchorCommitSha,
      body: body.trim(),
      kind,
    };
    if (anchorFilePath) reqBody.anchor_file_path = anchorFilePath;
    if (anchorLineStart != null) reqBody.anchor_line_start = anchorLineStart;
    if (anchorLineEnd != null) reqBody.anchor_line_end = anchorLineEnd;
    if (addressedTo.trim()) reqBody.addressed_to = addressedTo.trim();

    const { data, error } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/comments',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
        body: reqBody,
      },
    );

    submitting = false;
    if (error) {
      submitError = 'Failed to post comment.';
    } else if (data) {
      body = '';
      addressedTo = '';
      onsubmit?.(data);
    }
  }
</script>

<div class="composer" role="dialog" aria-label="Comment composer">
  <div class="composer-header">
    <span class="composer-title">Add comment</span>
    {#if anchorFilePath}
      <span class="anchor-label">
        {anchorFilePath}{anchorLineStart != null ? `:${anchorLineStart}` : ''}
        {anchorLineEnd != null && anchorLineEnd !== anchorLineStart ? `–${anchorLineEnd}` : ''}
      </span>
    {:else}
      <span class="anchor-label">{anchorCommitSha.slice(0, 7)}</span>
    {/if}
    <button class="close-btn" onclick={() => oncancel?.()} aria-label="Close comment composer">×</button>
  </div>

  <form class="composer-form" onsubmit={handleSubmit}>
    <div class="field-row">
      <label class="field-label" for="comment-kind">Kind</label>
      <select id="comment-kind" class="kind-select" bind:value={kind}>
        {#each KINDS as k}
          <option value={k}>{k}</option>
        {/each}
      </select>
    </div>

    <div class="field-row">
      <label class="field-label" for="addressed-to">Addressed to</label>
      <input
        id="addressed-to"
        class="addr-input"
        type="text"
        placeholder="@agent, @all-agents, or empty"
        bind:value={addressedTo}
      />
    </div>

    <textarea
      class="body-area"
      placeholder="Comment body (markdown supported)…"
      rows={4}
      bind:value={body}
      aria-label="Comment body"
    ></textarea>

    {#if submitError}
      <p class="error" role="alert">{submitError}</p>
    {/if}

    <div class="actions">
      <Button variant="ghost" size="sm" onclick={() => oncancel?.()}>Cancel</Button>
      <Button variant="accent" size="sm" type="submit" disabled={submitting || !body.trim()}>
        {submitting ? 'Posting…' : 'Post comment'}
      </Button>
    </div>
  </form>
</div>

<style>
  .composer {
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border-strong);
    border-radius: var(--radius-md);
    min-width: 320px;
    max-width: 480px;
    box-shadow: 0 8px 24px rgba(0, 0, 0, 0.18);
  }

  .composer-header {
    display: flex;
    align-items: center;
    gap: 8px;
    padding: 10px 14px;
    border-bottom: 1px solid var(--color-border);
    font-size: var(--font-size-sm);
  }

  .composer-title {
    font-weight: var(--font-weight-semibold);
    color: var(--color-text-primary);
  }

  .anchor-label {
    font-family: var(--font-mono);
    font-size: 10px;
    padding: 1px 6px;
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
    border-radius: 3px;
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .close-btn {
    background: transparent;
    border: 0;
    color: var(--color-text-secondary);
    font-size: 18px;
    cursor: pointer;
    padding: 0 2px;
    line-height: 1;
  }

  .close-btn:hover {
    color: var(--color-text-primary);
  }

  .composer-form {
    padding: 12px 14px;
    display: flex;
    flex-direction: column;
    gap: 10px;
  }

  .field-row {
    display: grid;
    grid-template-columns: 90px 1fr;
    align-items: center;
    gap: 8px;
  }

  .field-label {
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .kind-select {
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 4px 8px;
  }

  .addr-input {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 4px 8px;
  }

  .body-area {
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 8px;
    resize: vertical;
    line-height: 1.5;
    min-height: 80px;
  }

  .body-area:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: -1px;
    border-color: var(--color-accent);
  }

  .error {
    color: var(--color-danger);
    font-size: var(--font-size-sm);
    margin: 0;
  }

  .actions {
    display: flex;
    justify-content: flex-end;
    gap: 8px;
  }
</style>
