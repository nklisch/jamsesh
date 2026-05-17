import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import Badge from './Badge.svelte';

function textSnippet(text: string) {
  return createRawSnippet(() => ({
    render: () => `<span>${text}</span>`,
  }));
}

describe('Badge', () => {
  it('renders children', () => {
    render(Badge, { props: { children: textSnippet('sync') } });
    expect(screen.getByText('sync')).toBeInTheDocument();
  });

  it('defaults to variant=neutral', () => {
    render(Badge, { props: { children: textSnippet('test') } });
    const pill = screen.getByText('test').closest('.pill');
    expect(pill).toHaveClass('pill-neutral');
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
      render(Badge, { props: { variant, children: textSnippet(variant) } });
      const pill = screen.getByText(variant).closest('.pill');
      expect(pill).toHaveClass(`pill-${variant}`);
    });
  }

  it('always has the base pill class', () => {
    render(Badge, { props: { children: textSnippet('base') } });
    const pill = screen.getByText('base').closest('.pill');
    expect(pill).toHaveClass('pill');
  });
});
