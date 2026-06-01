<script lang="ts">
  import { client } from '$lib/api/client';

  let {
    sessionId,
    orgId,
    selectedSha,
    selectedPath,
    onrangeselect,
  }: {
    sessionId: string;
    orgId: string;
    selectedSha: string | null;
    selectedPath: string | null;
    onrangeselect?: (range: { start: number; end: number } | null) => void;
  } = $props();

  let content = $state('');
  let isBinary = $state(false);
  let mime = $state('');
  let loading = $state(false);
  let loadError = $state<string | null>(null);
  let selectedRange = $state<{ start: number; end: number } | null>(null);

  $effect(() => {
    if (!selectedSha || !selectedPath) {
      content = '';
      isBinary = false;
      mime = '';
      selectedRange = null;
      return;
    }
    loading = true;
    loadError = null;
    const controller = new AbortController();
    const sha = selectedSha;
    const filePath = selectedPath;
    client.GET('/api/orgs/{orgID}/sessions/{sessionID}/files', {
      params: {
        path: { orgID: orgId, sessionID: sessionId },
        query: { commit: sha, path: filePath },
      },
      signal: controller.signal,
    })
      .then(({ data, error, response }) => {
        if (controller.signal.aborted) return;
        if (error) {
          const message = (error as { message?: string }).message ?? `HTTP ${response.status}`;
          throw new Error(message);
        }
        if (!data) throw new Error('Failed to load file.');
        content = data.content;
        isBinary = data.is_binary;
        mime = data.mime;
      })
      .catch((e: unknown) => {
        if (controller.signal.aborted) return;
        loadError = e instanceof Error ? e.message : 'Failed to load file.';
      })
      .finally(() => {
        if (!controller.signal.aborted) loading = false;
      });
    return () => controller.abort();
  });

  // Reset selection when file changes.
  $effect(() => {
    void selectedPath;
    void selectedSha;
    selectedRange = null;
    onrangeselect?.(null);
  });

  function lineClick(n: number, e: MouseEvent) {
    if (e.shiftKey && selectedRange) {
      const start = Math.min(selectedRange.start, n);
      const end = Math.max(selectedRange.start, n);
      selectedRange = { start, end };
    } else {
      selectedRange = { start: n, end: n };
    }
    onrangeselect?.(selectedRange);
  }

  function isLineSelected(n: number): boolean {
    if (!selectedRange) return false;
    return n >= selectedRange.start && n <= selectedRange.end;
  }

  let lines = $derived(content ? content.split('\n') : []);
</script>

<div class="artifact-pane" aria-label="File viewer">
  {#if !selectedSha || !selectedPath}
    <div class="empty-state">
      <p class="empty-msg">Select a commit and file to view its content.</p>
    </div>
  {:else if loading}
    <div class="loading" aria-busy="true">Loading file…</div>
  {:else if loadError}
    <div class="error" role="alert">{loadError}</div>
  {:else if isBinary}
    <div class="binary-placeholder" aria-label="Binary file">
      <p class="binary-msg">Binary file ({mime}) — preview not available.</p>
    </div>
  {:else}
    <div class="file-header">
      <span class="file-path" title={selectedPath ?? ''}>{selectedPath}</span>
      <span class="sha-badge">{selectedSha?.slice(0, 7)}</span>
      {#if selectedRange}
        <span class="range-badge">
          {selectedRange.start}{selectedRange.start !== selectedRange.end ? `–${selectedRange.end}` : ''}
        </span>
      {/if}
    </div>
    <div class="code-scroll">
      <pre class="code-block" aria-label="File content"><code>{#each lines as line, i}{@const n = i + 1}<span
          class="code-line"
          class:selected={isLineSelected(n)}
          role="button"
          tabindex="0"
          aria-label="Line {n}"
          aria-pressed={isLineSelected(n)}
          onclick={(e) => lineClick(n, e)}
          onkeydown={(e) => e.key === 'Enter' && lineClick(n, e as unknown as MouseEvent)}
        ><span class="lineno" aria-hidden="true">{n}</span><span class="line-text">{line}</span></span>
{/each}</code></pre>
    </div>
  {/if}
</div>

<style>
  .artifact-pane {
    height: 100%;
    display: flex;
    flex-direction: column;
    overflow: hidden;
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
  }

  .empty-state,
  .loading,
  .error,
  .binary-placeholder {
    flex: 1;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 40px;
    text-align: center;
  }

  .empty-msg,
  .loading,
  .binary-msg {
    color: var(--color-text-tertiary);
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
  }

  .error {
    color: var(--color-danger);
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
  }

  /* ── File header ─────────────────────────────────────────── */
  .file-header {
    display: flex;
    align-items: center;
    gap: 10px;
    padding: 6px 14px;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
    flex-shrink: 0;
    font-size: 11px;
  }

  .file-path {
    font-family: var(--font-mono);
    color: var(--color-text-primary);
    flex: 1;
    overflow: hidden;
    text-overflow: ellipsis;
    white-space: nowrap;
  }

  .sha-badge,
  .range-badge {
    font-family: var(--font-mono);
    font-size: 10px;
    padding: 1px 6px;
    border-radius: 3px;
    background: var(--color-bg-tertiary);
    color: var(--color-text-secondary);
    white-space: nowrap;
    flex-shrink: 0;
  }

  .range-badge {
    background: var(--color-accent-muted);
    color: var(--color-accent);
  }

  /* ── Code block ──────────────────────────────────────────── */
  .code-scroll {
    flex: 1;
    overflow: auto;
  }

  .code-block {
    margin: 0;
    padding: 0;
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    line-height: 1.6;
    min-width: 100%;
    width: max-content;
  }

  .code-line {
    display: flex;
    align-items: flex-start;
    cursor: pointer;
    user-select: none;
    border-left: 3px solid transparent;
  }

  .code-line:hover {
    background: var(--color-bg-secondary);
  }

  .code-line.selected {
    background: var(--color-accent-muted);
    border-left-color: var(--color-accent);
  }

  .lineno {
    display: inline-block;
    min-width: 48px;
    padding: 0 12px 0 8px;
    text-align: right;
    color: var(--color-text-tertiary);
    font-size: 11px;
    user-select: none;
    flex-shrink: 0;
  }

  .line-text {
    white-space: pre;
    padding: 0 8px;
    flex: 1;
  }
</style>
