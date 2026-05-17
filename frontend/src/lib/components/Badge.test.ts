import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import Badge from './Badge.svelte';

describe('Badge', () => {
  it('renders children', () => {
    render(Badge, { props: { children: () => 'sync' } });
    expect(screen.getByText('sync')).toBeInTheDocument();
  });

  it('defaults to variant=neutral', () => {
    render(Badge, { props: { children: () => 'test' } });
    const el = screen.getByText('test');
    expect(el).toHaveClass('pill-neutral');
  });

  const variants = [
    'neutral',
    'success',
    'warning',
    'danger',
    'accent',
    'sync',
    'isolated',
    'conflict',
  ] as const;

  for (const variant of variants) {
    it(`applies pill-${variant} class for variant="${variant}"`, () => {
      render(Badge, { props: { variant, children: () => variant } });
      expect(screen.getByText(variant)).toHaveClass(`pill-${variant}`);
    });
  }

  it('always has the base pill class', () => {
    render(Badge, { props: { children: () => 'base' } });
    expect(screen.getByText('base')).toHaveClass('pill');
  });
});
