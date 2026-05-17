import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import ModePill from './ModePill.svelte';

describe('ModePill', () => {
  it('renders "sync" text for mode="sync"', () => {
    render(ModePill, { props: { mode: 'sync' } });
    expect(screen.getByText('sync')).toBeInTheDocument();
  });

  it('renders "isolated" text for mode="isolated"', () => {
    render(ModePill, { props: { mode: 'isolated' } });
    expect(screen.getByText('isolated')).toBeInTheDocument();
  });

  it('applies sync styling via Badge variant for mode="sync"', () => {
    render(ModePill, { props: { mode: 'sync' } });
    // The inner Badge renders a span with pill-sync class
    expect(screen.getByText('sync')).toHaveClass('pill-sync');
  });

  it('applies isolated styling via Badge variant for mode="isolated"', () => {
    render(ModePill, { props: { mode: 'isolated' } });
    expect(screen.getByText('isolated')).toHaveClass('pill-isolated');
  });
});
