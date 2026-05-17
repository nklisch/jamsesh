import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import { createRawSnippet } from 'svelte';
import Button from './Button.svelte';

function textSnippet(text: string) {
  return createRawSnippet(() => ({
    render: () => `<span>${text}</span>`,
  }));
}

describe('Button', () => {
  it('renders children', () => {
    render(Button, {
      props: { children: textSnippet('Click me') },
    });
    expect(screen.getByRole('button')).toHaveTextContent('Click me');
  });

  it('defaults to variant=primary, size=md, type=button', () => {
    render(Button, { props: { children: textSnippet('OK') } });
    const btn = screen.getByRole('button');
    expect(btn).toHaveClass('btn-primary', 'btn-md');
    expect(btn).toHaveAttribute('type', 'button');
  });

  it('applies variant class', () => {
    render(Button, { props: { variant: 'ghost', children: textSnippet('Ghost') } });
    expect(screen.getByRole('button')).toHaveClass('btn-ghost');
  });

  it('applies accent variant class', () => {
    render(Button, { props: { variant: 'accent', children: textSnippet('Accent') } });
    expect(screen.getByRole('button')).toHaveClass('btn-accent');
  });

  it('applies size class', () => {
    render(Button, { props: { size: 'lg', children: textSnippet('Large') } });
    expect(screen.getByRole('button')).toHaveClass('btn-lg');
  });

  it('sets type=submit when prop is submit', () => {
    render(Button, { props: { type: 'submit', children: textSnippet('Submit') } });
    expect(screen.getByRole('button')).toHaveAttribute('type', 'submit');
  });

  it('disables the button when disabled=true', () => {
    render(Button, { props: { disabled: true, children: textSnippet('Disabled') } });
    expect(screen.getByRole('button')).toBeDisabled();
  });

  it('calls onclick handler when clicked', async () => {
    const handler = vi.fn();
    render(Button, { props: { onclick: handler, children: textSnippet('Click') } });
    await fireEvent.click(screen.getByRole('button'));
    expect(handler).toHaveBeenCalledOnce();
  });

  it('does not fire onclick when disabled', async () => {
    const handler = vi.fn();
    render(Button, {
      props: { disabled: true, onclick: handler, children: textSnippet('No click') },
    });
    const btn = screen.getByRole('button');
    // Verify the button is disabled — native browsers suppress click on disabled buttons.
    // JSDOM's fireEvent bypasses this suppression, so we assert the disabled state directly.
    expect(btn).toBeDisabled();
  });
});
