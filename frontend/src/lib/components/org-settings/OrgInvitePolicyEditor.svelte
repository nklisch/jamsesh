<script lang="ts">
  import { untrack } from 'svelte';
  import { client } from '$lib/api/client';
  import type { components } from '$lib/api/types.gen';

  type Org = components['schemas']['Org'];
  type OrgSessionInvitePolicy = components['schemas']['OrgSessionInvitePolicy'];

  let {
    orgId,
    org,
    isAdmin,
    onorgchanged,
  }: {
    orgId: string;
    org: Org;
    isAdmin: boolean;
    onorgchanged?: (updated: Org) => void;
  } = $props();

  // ── Editing state ──────────────────────────────────────────────────────────
  // Seed from the prop's initial value using untrack so Svelte does not
  // subscribe to `org` here — these are independent local edit state variables.
  let selectedPolicy = $state<OrgSessionInvitePolicy>(
    untrack(() => org.session_invite_policy),
  );
  let savedPolicy = $state<OrgSessionInvitePolicy>(
    untrack(() => org.session_invite_policy),
  );
  let isDirty = $derived(selectedPolicy !== savedPolicy);

  // ── Save state ─────────────────────────────────────────────────────────────
  let saving = $state(false);
  let saveError = $state<string | null>(null);
  let saveSuccess = $state(false);
  let saveSuccessTimer: ReturnType<typeof setTimeout> | null = null;

  // ── Handlers ───────────────────────────────────────────────────────────────
  async function save(): Promise<void> {
    if (!isDirty || saving) return;
    saving = true;
    saveError = null;
    saveSuccess = false;

    const { data, error } = await client.PATCH('/api/orgs/{orgID}', {
      params: { path: { orgID: orgId } },
      body: { session_invite_policy: selectedPolicy },
    });

    saving = false;

    if (error) {
      const anyError = error as { error?: string; message?: string };
      if (anyError.error === 'auth.insufficient_permission') {
        saveError = 'Only org creators can change this setting.';
      } else {
        saveError = anyError.message ?? 'Failed to save. Please try again.';
      }
      return;
    }

    if (data) {
      savedPolicy = data.session_invite_policy;
      selectedPolicy = data.session_invite_policy;
      onorgchanged?.(data);
    }

    saveSuccess = true;
    if (saveSuccessTimer !== null) clearTimeout(saveSuccessTimer);
    saveSuccessTimer = setTimeout(() => {
      saveSuccess = false;
    }, 2000);
  }

  function discard(): void {
    selectedPolicy = savedPolicy;
    saveError = null;
  }
</script>

<main class="pane">
  <h1>Session invites</h1>
  <p class="pane-sub">
    Configure who can be added to sessions in <strong>{org.name}</strong>.
  </p>

  {#if !isAdmin}
    <div class="readonly-note" role="note">
      Only org creators can change this setting. You're viewing it as a member.
    </div>
  {/if}

  {#if saveError}
    <div class="save-error-banner" role="alert">{saveError}</div>
  {/if}

  <div class="field">
    <div class="field-label">Policy</div>
    <div class="field-desc">
      Sets who is allowed to accept a session invite. Affects future invite-accept
      attempts only; existing session members are grandfathered when the policy changes.
    </div>

    <label
      class="opt"
      class:selected={selectedPolicy === 'members_only'}
      class:disabled={!isAdmin}
    >
      <input
        type="radio"
        name="policy"
        value="members_only"
        bind:group={selectedPolicy}
        disabled={!isAdmin}
      />
      <div>
        <div class="opt-label">
          Members only
          <span class="opt-tag">recommended</span>
        </div>
        <div class="opt-desc">
          Invitee must already be an org member. Strictest; matches the multi-tenancy
          default.
        </div>
      </div>
    </label>

    <label
      class="opt"
      class:selected={selectedPolicy === 'open'}
      class:disabled={!isAdmin}
    >
      <input
        type="radio"
        name="policy"
        value="open"
        bind:group={selectedPolicy}
        disabled={!isAdmin}
      />
      <div>
        <div class="opt-label">Open</div>
        <div class="opt-desc">
          Anyone with a valid email invite can join as a session-only guest. They get
          access to that session only — not to other org resources.
        </div>
      </div>
    </label>
  </div>

  <div class="pane-actions">
    {#if saveSuccess}
      <span class="save-success" role="status">Saved</span>
    {/if}
    <button
      class="btn ghost"
      type="button"
      onclick={discard}
      disabled={!isDirty || !isAdmin || saving}
    >
      Discard
    </button>
    <button
      class="btn primary"
      type="button"
      onclick={save}
      disabled={!isDirty || !isAdmin || saving}
      aria-busy={saving}
    >
      {saving ? 'Saving…' : 'Save changes'}
    </button>
  </div>
</main>

<style>
  /* ── Content pane ────────────────────────────────────────────────── */
  .pane {
    padding: 32px 40px;
    max-width: 720px;
  }

  .pane h1 {
    margin: 0 0 4px;
    font-size: var(--font-size-xl);
    font-weight: var(--font-weight-semibold);
    letter-spacing: -0.02em;
  }

  .pane-sub {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    margin-bottom: 28px;
    margin-top: 0;
    line-height: 1.5;
  }

  /* ── Banners ─────────────────────────────────────────────────────── */
  .readonly-note {
    margin-bottom: 16px;
    padding: 10px 14px;
    background: var(--color-warning-muted);
    border-left: 3px solid var(--color-warning);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
  }

  .save-error-banner {
    margin-bottom: 16px;
    padding: 10px 14px;
    background: var(--color-warning-muted);
    border-left: 3px solid var(--color-warning);
    font-size: var(--font-size-sm);
    color: var(--color-text-secondary);
    border-radius: 0 var(--radius-md) var(--radius-md) 0;
  }

  /* ── Field ───────────────────────────────────────────────────────── */
  .field {
    margin-bottom: 20px;
  }

  .field-label {
    font-weight: var(--font-weight-medium);
    font-size: var(--font-size-base);
    margin-bottom: 4px;
  }

  .field-desc {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    margin-bottom: 12px;
    line-height: 1.5;
  }

  /* ── Radio options ───────────────────────────────────────────────── */
  .opt {
    display: flex;
    gap: 12px;
    align-items: flex-start;
    padding: 12px 14px;
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    background: var(--color-bg-secondary);
    cursor: pointer;
    transition: border-color 0.15s, background 0.15s;
    margin-bottom: 8px;
  }

  .opt:hover:not(.disabled) {
    border-color: var(--color-border-strong);
  }

  .opt.selected {
    border-color: var(--color-accent);
    background: var(--color-accent-muted);
  }

  .opt.disabled {
    cursor: not-allowed;
    opacity: 0.7;
  }

  .opt input[type='radio'] {
    margin-top: 2px;
    accent-color: var(--color-accent);
    flex-shrink: 0;
  }

  .opt-label {
    font-weight: var(--font-weight-medium);
    font-size: var(--font-size-base);
  }

  .opt-tag {
    font-size: var(--font-size-xs);
    color: var(--color-text-tertiary);
    font-weight: 400;
    margin-left: 6px;
  }

  .opt-desc {
    color: var(--color-text-secondary);
    font-size: var(--font-size-sm);
    line-height: 1.5;
    margin-top: 4px;
  }

  /* ── Pane actions ────────────────────────────────────────────────── */
  .pane-actions {
    display: flex;
    justify-content: flex-end;
    align-items: center;
    gap: 8px;
    padding-top: 24px;
    border-top: 1px solid var(--color-border);
    margin-top: 32px;
  }

  .save-success {
    font-size: var(--font-size-sm);
    color: var(--color-success);
    font-weight: var(--font-weight-medium);
    margin-right: auto;
  }

  .btn {
    border-radius: var(--radius-md);
    padding: 6px 14px;
    font-size: var(--font-size-sm);
    font-weight: var(--font-weight-medium);
    border: 1px solid transparent;
    cursor: pointer;
    font-family: var(--font-sans);
  }

  .btn.primary {
    background: var(--color-accent);
    color: #fff;
    border-color: var(--color-accent);
  }

  .btn.primary:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }

  .btn.ghost {
    background: transparent;
    color: var(--color-text-primary);
    border-color: var(--color-border-strong);
  }

  .btn.ghost:disabled {
    opacity: 0.4;
    cursor: not-allowed;
  }
</style>
