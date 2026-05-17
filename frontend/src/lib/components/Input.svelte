<script lang="ts">
  let {
    value = $bindable(''),
    type = 'text',
    placeholder = '',
    disabled = false,
    oninput,
  }: {
    value?: string;
    type?: 'text' | 'email';
    placeholder?: string;
    disabled?: boolean;
    oninput?: (e: Event) => void;
  } = $props();
</script>

<input
  class="input"
  {type}
  bind:value
  {placeholder}
  {disabled}
  {oninput}
/>

<style>
  .input {
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    line-height: var(--line-height-base);
    color: var(--color-text-primary);
    background: var(--color-bg-secondary);
    border: 1px solid var(--color-border);
    border-radius: var(--radius-md);
    padding: var(--space-2) var(--space-3);
    width: 100%;
    transition: border-color 120ms ease, outline 120ms ease;
  }

  .input::placeholder {
    color: var(--color-text-tertiary);
  }

  .input:focus {
    outline: 2px solid var(--color-accent);
    outline-offset: -1px;
    border-color: var(--color-accent);
  }

  .input:disabled {
    opacity: 0.5;
    cursor: not-allowed;
    background: var(--color-bg-tertiary);
  }
</style>
