import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import InlineCode from './InlineCode.svelte';

describe('InlineCode', () => {
  it('renders children inside a <code> element', () => {
    render(InlineCode, { props: { children: () => 'console.log()' } });
    const el = screen.getByText('console.log()');
    expect(el.tagName.toLowerCase()).toBe('code');
  });

  it('has the inline-code class', () => {
    render(InlineCode, { props: { children: () => 'some code' } });
    expect(screen.getByText('some code')).toHaveClass('inline-code');
  });

  it('renders arbitrary text content', () => {
    render(InlineCode, { props: { children: () => 'git commit -am "fix"' } });
    expect(screen.getByText('git commit -am "fix"')).toBeInTheDocument();
  });
});
