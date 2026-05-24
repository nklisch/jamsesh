<script lang="ts">
  // PlaygroundChip — small badge rendered in the app-chrome header when a
  // session belongs to the reserved playground org ('org_playground').
  // Visual: amber pill with a clock icon, uppercase monospace label.
  // Matches the `.playground-chip` style from mockup 03-creator-session.html.
  // Style-only component — no props or reactive state.
</script>

<span class="playground-chip" aria-label="Playground session">
  <span class="icon" aria-hidden="true">⏳</span>
  <span class="label">playground</span>
</span>

<style>
  .playground-chip {
    display: inline-flex;
    align-items: center;
    gap: 6px;
    padding: 3px 9px;
    background: var(--color-warning-muted);
    color: var(--color-warning);
    border: 1px solid rgba(148, 101, 35, 0.25);
    border-radius: var(--radius-md);
    font: var(--font-weight-semibold) 10px var(--font-mono);
    text-transform: uppercase;
    letter-spacing: 0.06em;
    white-space: nowrap;
    user-select: none;
  }

  .icon {
    font-size: 11px;
    line-height: 1;
  }
</style>
