<script lang="ts">
  import AuthorDot from '$lib/components/AuthorDot.svelte';

  type SourceCommit = {
    sha: string;
    author_name: string;
    author_id: string;
    subject: string;
  };
  type RefGroup = {
    ref: string;
    kind: 'draft' | 'isolated' | 'sync';
    tip_sha: string;
    commits: SourceCommit[];
  };

  type Props = {
    refs: RefGroup[];
    selected: Set<string>;
    onToggle: (sha: string) => void;
    onAddAll?: (group: RefGroup) => void;
    disabled?: boolean;
  };

  let { refs, selected, onToggle, onAddAll, disabled = false }: Props = $props();

  function shortRefName(ref: string): string {
    return ref.replace(/^refs\/heads\//, '');
  }
</script>

<div class="panel-head">
  <h2>Available commits</h2>
  <div class="meta">{refs.length} refs</div>
</div>
<div class="panel-body">
  {#each refs as group (group.ref)}
    <div class="group-card">
      <header class="group-head">
        <span class="name">{shortRefName(group.ref)}</span>
        <span class="tag {group.kind}">{group.kind}</span>
        <span class="count">{group.commits.length} commit{group.commits.length === 1 ? '' : 's'}</span>
        <span class="actions">
          <button type="button" disabled={disabled} onclick={() => onAddAll?.(group)}>
            Add all
          </button>
        </span>
      </header>
      {#each group.commits as c (c.sha)}
        {@const inCart = selected.has(c.sha)}
        <div class="commit-row" class:in-cart={inCart}>
          <div class="body">
            <div class="top">
              <AuthorDot authorId={c.author_id} size={9} />
              <span class="name">{c.author_name}</span>
              <span class="sha">{c.sha.slice(0, 7)}</span>
            </div>
            <div class="msg">{c.subject}</div>
          </div>
          <button
            class="add-btn"
            type="button"
            aria-label={inCart ? `Remove ${c.sha.slice(0, 7)}` : `Add ${c.sha.slice(0, 7)}`}
            disabled={disabled}
            onclick={() => onToggle(c.sha)}
          >{inCart ? '−' : '+'}</button>
        </div>
      {/each}
    </div>
  {:else}
    <p class="empty">No refs available yet.</p>
  {/each}
</div>

<style>
  .panel-head {
    padding: 14px 22px;
    display: flex; align-items: center; gap: 12px;
    border-bottom: 1px solid var(--color-border);
    background: var(--color-bg-secondary);
  }
  .panel-head h2 {
    margin: 0; font-size: var(--font-size-base);
    font-weight: var(--font-weight-semibold);
  }
  .panel-head .meta {
    font: var(--font-weight-medium) var(--font-size-sm) var(--font-mono);
    color: var(--color-text-secondary); margin-left: auto;
  }

  .panel-body { overflow: auto; padding: 8px 14px 24px; flex: 1; }

  .group-card {
    margin: 10px 0;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-bg-secondary);
    overflow: hidden;
  }
  .group-head {
    padding: 10px 14px;
    display: flex; gap: 10px; align-items: center;
    background: var(--color-bg-tertiary);
    border-bottom: 1px solid var(--color-border);
  }
  .group-head .name {
    font: var(--font-weight-semibold) var(--font-size-sm) var(--font-mono);
    color: var(--color-text-primary);
  }
  .group-head .tag {
    font: 10px var(--font-mono); text-transform: uppercase; letter-spacing: 0.08em;
    padding: 2px 7px; border-radius: var(--radius-full);
  }
  .group-head .tag.draft { background: var(--color-accent-muted); color: var(--color-accent); }
  .group-head .tag.isolated { background: var(--color-warning-muted); color: var(--color-warning); }
  .group-head .tag.sync { background: var(--color-bg-tertiary); color: var(--color-text-secondary); }
  .group-head .count {
    margin-left: auto;
    font: var(--font-size-xs) var(--font-mono); color: var(--color-text-tertiary);
  }
  .group-head .actions { margin-left: 8px; }
  .group-head button {
    border: 1px solid var(--color-border); background: var(--color-bg-secondary);
    padding: 3px 8px; border-radius: var(--radius-sm);
    font: var(--font-weight-medium) 11px var(--font-sans);
    color: var(--color-text-primary); cursor: pointer;
  }
  .group-head button:hover:not(:disabled) { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .group-head button:disabled { opacity: 0.5; cursor: not-allowed; }

  .commit-row {
    padding: 9px 14px;
    display: grid; grid-template-columns: 1fr auto; gap: 8px;
    align-items: center;
    border-bottom: 1px solid var(--color-border);
  }
  .commit-row:last-child { border-bottom: 0; }
  .commit-row.in-cart { background: var(--color-accent-muted); }
  .commit-row .body { min-width: 0; }
  .commit-row .top {
    display: flex; gap: 7px; align-items: center;
    font: var(--font-size-xs) var(--font-mono);
    color: var(--color-text-tertiary);
    margin-bottom: 3px;
  }
  .commit-row .top .name { color: var(--color-text-secondary); font-weight: var(--font-weight-medium); }
  .commit-row .top .sha { color: var(--color-text-secondary); }
  .commit-row .msg {
    font-size: var(--font-size-sm); color: var(--color-text-primary);
    line-height: 1.35;
    overflow: hidden; text-overflow: ellipsis; white-space: nowrap;
  }
  .add-btn {
    width: 26px; height: 26px;
    border: 1px solid var(--color-border-strong);
    background: var(--color-bg-primary); color: var(--color-text-primary);
    border-radius: var(--radius-sm); cursor: pointer;
    font: var(--font-weight-semibold) 14px var(--font-sans);
    line-height: 1;
  }
  .add-btn:hover:not(:disabled) { background: var(--color-bg-inverse); color: var(--color-text-inverse); }
  .add-btn:disabled { opacity: 0.5; cursor: not-allowed; }

  .empty { color: var(--color-text-secondary); font-size: var(--font-size-sm); }
</style>
