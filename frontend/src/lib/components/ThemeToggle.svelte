<script lang="ts">
  import { onMount } from 'svelte';

  type Theme = 'system' | 'light' | 'dark';

  const STORAGE_KEY = 'jamsesh.theme';
  const CYCLE: Record<Theme, Theme> = {
    system: 'light',
    light: 'dark',
    dark: 'system',
  };

  let theme = $state<Theme>('system');

  onMount(() => {
    const saved = localStorage.getItem(STORAGE_KEY) as Theme | null;
    if (saved === 'light' || saved === 'dark' || saved === 'system') {
      theme = saved;
      applyTheme(saved);
    }
  });

  function applyTheme(t: Theme) {
    const root = document.documentElement;
    if (t === 'system') {
      root.removeAttribute('data-theme');
    } else {
      root.setAttribute('data-theme', t);
    }
  }

  function cycle() {
    theme = CYCLE[theme];
    localStorage.setItem(STORAGE_KEY, theme);
    applyTheme(theme);
  }

  const LABEL: Record<Theme, string> = {
    system: 'System theme',
    light: 'Light theme',
    dark: 'Dark theme',
  };

  const ICON: Record<Theme, string> = {
    system: '⏾',
    light: '☀',
    dark: '●',
  };
</script>

<button
  class="theme-toggle"
  onclick={cycle}
  aria-label={LABEL[theme]}
  title={LABEL[theme]}
>
  <span aria-hidden="true">{ICON[theme]}</span>
</button>

<style>
  .theme-toggle {
    display: inline-flex;
    align-items: center;
    justify-content: center;
    background: transparent;
    border: 1px solid var(--color-border);
    color: var(--color-text-secondary);
    padding: var(--space-1) var(--space-2);
    border-radius: var(--radius-md);
    cursor: pointer;
    font-family: var(--font-sans);
    font-size: var(--font-size-base);
    line-height: 1;
    transition: background-color 120ms ease;
  }

  .theme-toggle:hover {
    background: var(--color-bg-tertiary);
  }
</style>
