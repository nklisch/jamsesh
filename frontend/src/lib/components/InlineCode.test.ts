import { describe, it, expect } from 'vitest';
import { render, screen } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import InlineCode from './InlineCode.svelte';

function textSnippet(text: string) {
  return createRawSnippet(() => ({
    render: () => `<span>${text}</span>`,
  }));
}

describe('InlineCode', () => {
  it('renders children inside a <code> element', () => {
    render(InlineCode, { props: { children: textSnippet('console.log()') } });
    // The component wraps children in <code class="inline-code">
    const codeEl = document.querySelector('code.inline-code');
    expect(codeEl).toBeInTheDocument();
    expect(codeEl?.textContent).toBe('console.log()');
  });

  it('has the inline-code class', () => {
    render(InlineCode, { props: { children: textSnippet('some code') } });
    expect(document.querySelector('code.inline-code')).toBeInTheDocument();
  });

  it('renders arbitrary text content', () => {
    render(InlineCode, { props: { children: textSnippet('git commit -am "fix"') } });
    expect(screen.getByText('git commit -am "fix"')).toBeInTheDocument();
  });
});
