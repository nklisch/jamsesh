<script lang="ts">
  // SessionAttachWalkthrough — tiered-disclosure attach modal.
  //
  // Mode state machine (view-state-union-machine pattern):
  //   'full'    → first-time ceremonial (two shell installs + CC join)
  //   'compact' → returning user (CC pane only + "First-time setup?" link)
  //
  // Mode is read from localStorage on mount (and whenever open flips true).
  // "Don't show again" checkbox in full mode persists the flag on close.

  import FullCard from './walkthrough/FullCard.svelte';
  import CompactCard from './walkthrough/CompactCard.svelte';

  // ── State machine ──────────────────────────────────────────────────────────
  //
  //  full    → compact  (localStorage flag set + re-open)
  //  compact → full     (user clicks "First-time setup?" link)
  type Mode = 'full' | 'compact';

  type Props = {
    open: boolean;
    sessionId: string | null;      // null = chrome-help mode (no specific session)
    onclose: () => void;
    onopenSession?: () => void;
  };

  let { open, sessionId, onclose, onopenSession }: Props = $props();

  // ── Constants ──────────────────────────────────────────────────────────────
  const DISMISS_KEY = 'jamsesh.attach-walkthrough-dismissed';

  // Composed at render time from sessionId.
  let joinCmd = $derived(sessionId ? `/jamsesh:join ${sessionId}` : null);

  // ── Internal state ─────────────────────────────────────────────────────────
  let mode = $state<Mode>('full');
  let dismissChecked = $state(false);
  let copiedCmd = $state<string | null>(null);

  // DOM ref for focus management.
  let closeBtn = $state<HTMLButtonElement | null>(null);

  // ── Mode-on-mount: re-read flag whenever open flips true ──────────────────
  $effect(() => {
    if (!open) return;
    mode =
      typeof localStorage !== 'undefined' &&
      localStorage.getItem(DISMISS_KEY) === 'true'
        ? 'compact'
        : 'full';
  });

  // ── ESC handler — registered while open, torn down on close ───────────────
  $effect(() => {
    if (!open) return;
    function handleKeyDown(e: KeyboardEvent) {
      if (e.key === 'Escape') {
        handleClose();
      }
    }
    window.addEventListener('keydown', handleKeyDown);
    return () => window.removeEventListener('keydown', handleKeyDown);
  });

  // ── Focus management — move focus to Close button on open, restore on close
  $effect(() => {
    if (open) {
      const prev = document.activeElement as HTMLElement | null;
      const id = requestAnimationFrame(() => {
        closeBtn?.focus();
      });
      return () => {
        cancelAnimationFrame(id);
        prev?.focus();
      };
    }
  });

  // ── Behaviors ──────────────────────────────────────────────────────────────

  /** Persist dismiss flag if checked, then call parent onclose. */
  function handleClose() {
    if (dismissChecked) {
      localStorage.setItem(DISMISS_KEY, 'true');
    }
    onclose();
  }

  /** "Open session view →" button handler. */
  function handleOpenSession() {
    if (dismissChecked) {
      localStorage.setItem(DISMISS_KEY, 'true');
    }
    if (onopenSession) {
      onopenSession();
    } else {
      onclose();
    }
  }

  /** Click-to-copy with ~1.2s "Copied" feedback badge. */
  async function copyCmd(cmd: string) {
    await navigator.clipboard.writeText(cmd);
    copiedCmd = cmd;
    setTimeout(() => {
      if (copiedCmd === cmd) copiedCmd = null;
    }, 1200);
  }
</script>

{#if open}
  <!-- Backdrop is a click-to-dismiss scrim, NOT the dialog landmark. The
       `role="dialog"` + aria-modal + aria-label live on the inner <article> in
       FullCard / CompactCard so screen-reader landmark navigation lands on the
       content rather than the scrim. (idea-attach-onboarding-dialog-role-on-card) -->
  <!-- svelte-ignore a11y_click_events_have_key_events -->
  <!-- svelte-ignore a11y_no_static_element_interactions -->
  <div
    class="modal-backdrop"
    role="presentation"
    onclick={(e) => {
      if (e.target === e.currentTarget) handleClose();
    }}
  >
    {#if mode === 'full'}
      <FullCard
        {sessionId}
        {joinCmd}
        {copiedCmd}
        {dismissChecked}
        bind:closeBtnRef={closeBtn}
        oncopy={copyCmd}
        ondismisschange={(checked) => (dismissChecked = checked)}
        onclose={handleClose}
        onopensession={handleOpenSession}
      />
    {:else}
      <CompactCard
        {sessionId}
        {joinCmd}
        {copiedCmd}
        bind:closeBtnRef={closeBtn}
        oncopy={copyCmd}
        onshowfull={() => (mode = 'full')}
        onclose={handleClose}
        onopensession={handleOpenSession}
      />
    {/if}
  </div>
{/if}

<style>
  /* Modal backdrop — same in both states */
  .modal-backdrop {
    position: fixed;
    inset: 56px 0 0 0;
    background: rgba(20, 22, 26, 0.55);
    z-index: 50;
    display: flex;
    align-items: center;
    justify-content: center;
    padding: 24px;
  }
</style>
