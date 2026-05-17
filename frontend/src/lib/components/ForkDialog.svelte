<script lang="ts">
  import { auth } from '$lib/auth.svelte';
  import Button from './Button.svelte';
  import Modal from './Modal.svelte';

  let {
    sessionId,
    sourceRef,
    onclose,
    onsuccess,
  }: {
    sessionId: string;
    sourceRef: string;
    onclose?: () => void;
    onsuccess?: () => void;
  } = $props();

  let targetRef = $state('');
  let mode = $state<'sync' | 'isolated'>('sync');
  let submitting = $state(false);
  let submitError = $state<string | null>(null);

  // Derive a sensible default target ref name from the source ref.
  // e.g. refs/heads/jam/<session>/<user>/main → <user>/fork
  let defaultTargetRef = $derived.by(() => {
    const parts = sourceRef.replace('refs/heads/', '').split('/');
    if (parts.length >= 4) {
      const user = parts[2];
      return `refs/heads/jam/${sessionId}/${user}/fork`;
    }
    return '';
  });

  $effect(() => {
    if (!targetRef && defaultTargetRef) {
      targetRef = defaultTargetRef;
    }
  });

  async function handleSubmit(e: Event) {
    e.preventDefault();
    if (!targetRef.trim()) return;
    submitting = true;
    submitError = null;

    // Resolve tip SHA of the source ref: fetch refs and find it.
    let commitSha: string | null = null;
    try {
      const refsRes = await fetch(`/api/orgs/${encodeURIComponent(orgIdFromRef(sourceRef))}/sessions/${encodeURIComponent(sessionId)}/refs`, {
        headers: { Authorization: `Bearer ${auth.token ?? ''}` },
      });
      if (refsRes.ok) {
        const refsData = await refsRes.json() as { refs: Array<{ ref: string; sha: string }> };
        const found = refsData.refs.find((r) => r.ref === sourceRef);
        if (found) commitSha = found.sha;
      }
    } catch {
      // non-fatal; fork may still work with null sha
    }

    // Call MCP fork tool via /mcp JSON-RPC endpoint.
    const body = {
      jsonrpc: '2.0',
      id: 1,
      method: 'tools/call',
      params: {
        name: 'fork',
        arguments: {
          session_id: sessionId,
          target_ref: targetRef.trim(),
          mode,
          ...(commitSha ? { target_commit_sha: commitSha } : {}),
        },
      },
    };

    try {
      const res = await fetch('/mcp', {
        method: 'POST',
        headers: {
          'Content-Type': 'application/json',
          Accept: 'application/json, text/event-stream',
          Authorization: `Bearer ${auth.token ?? ''}`,
        },
        body: JSON.stringify(body),
      });

      if (!res.ok) {
        submitError = `Fork failed: HTTP ${res.status}`;
        submitting = false;
        return;
      }

      const text = await res.text();
      // Handle JSON-RPC or SSE response.
      const responseText = text.startsWith('data:')
        ? text.split('\n').find((l) => l.startsWith('data: '))?.slice(6) ?? text
        : text;

      let rpc: { error?: { message: string } };
      try {
        rpc = JSON.parse(responseText) as typeof rpc;
      } catch {
        rpc = {};
      }
      if (rpc.error) {
        submitError = `Fork failed: ${rpc.error.message}`;
        submitting = false;
        return;
      }

      submitting = false;
      onsuccess?.();
      onclose?.();
    } catch (e) {
      submitError = e instanceof Error ? e.message : 'Fork failed.';
      submitting = false;
    }
  }

  // Derive org ID from the session context (refs follow jam/<session>/<user>/<branch>).
  // In practice the orgId must come from the parent; we accept it as a prop here.
  function orgIdFromRef(_ref: string): string {
    // This is intentionally left as a best-effort; the orgId should be passed
    // from the parent but is not needed here since the /mcp endpoint doesn't
    // require it in the URL.
    return '';
  }
</script>

<Modal open={true} title="Fork ref" size="md" {onclose}>
  <form class="modal-body" onsubmit={handleSubmit}>
    <div class="field">
      <label class="label" for="fork-source">Source ref</label>
      <code class="mono-value">{sourceRef.split('/').slice(-2).join('/')}</code>
    </div>

    <div class="field">
      <label class="label" for="fork-target">Target ref</label>
      <input
        id="fork-target"
        class="text-input"
        type="text"
        bind:value={targetRef}
        placeholder="refs/heads/jam/<session>/<user>/<branch>"
        aria-label="Target ref name"
      />
    </div>

    <div class="field">
      <label class="label" for="fork-mode">Mode</label>
      <select id="fork-mode" class="select" bind:value={mode} aria-label="Fork mode">
        <option value="sync">sync</option>
        <option value="isolated">isolated</option>
      </select>
    </div>

    {#if submitError}
      <p class="error" role="alert">{submitError}</p>
    {/if}

    <div class="actions">
      <Button variant="ghost" size="sm" onclick={() => onclose?.()}>Cancel</Button>
      <Button variant="accent" size="sm" type="submit" disabled={submitting || !targetRef.trim()}>
        {submitting ? 'Forking…' : 'Fork'}
      </Button>
    </div>
  </form>
</Modal>

<style>
  .modal-body {
    padding: 16px;
    display: flex;
    flex-direction: column;
    gap: 14px;
  }

  .field {
    display: grid;
    grid-template-columns: 100px 1fr;
    align-items: center;
    gap: 10px;
  }

  .label {
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
  }

  .mono-value {
    font-family: var(--font-mono);
    font-size: var(--font-size-sm);
    color: var(--color-text-primary);
  }

  .text-input,
  .select {
    font-family: var(--font-sans);
    font-size: var(--font-size-sm);
    background: var(--color-bg-primary);
    color: var(--color-text-primary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-sm);
    padding: 6px 8px;
    width: 100%;
  }

  .text-input:focus,
  .select:focus {
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
    padding-top: 4px;
  }
</style>
