import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Button from './Button.svelte';

describe('Button', () => {
  it('renders children', () => {
    render(Button, {
      props: { children: () => 'Click me' },
    });
    expect(screen.getByRole('button')).toHaveTextContent('Click me');
  });

  it('defaults to variant=primary, size=md, type=button', () => {
    render(Button, { props: { children: () => 'OK' } });
    const btn = screen.getByRole('button');
    expect(btn).toHaveClass('btn-primary', 'btn-md');
    expect(btn).toHaveAttribute('type', 'button');
  });

  it('applies variant class', () => {
    render(Button, { props: { variant: 'ghost', children: () => 'Ghost' } });
    expect(screen.getByRole('button')).toHaveClass('btn-ghost');
  });

  it('applies accent variant class', () => {
    render(Button, { props: { variant: 'accent', children: () => 'Accent' } });
    expect(screen.getByRole('button')).toHaveClass('btn-accent');
  });

  it('applies size class', () => {
    render(Button, { props: { size: 'lg', children: () => 'Large' } });
    expect(screen.getByRole('button')).toHaveClass('btn-lg');
  });

  it('sets type=submit when prop is submit', () => {
    render(Button, { props: { type: 'submit', children: () => 'Submit' } });
    expect(screen.getByRole('button')).toHaveAttribute('type', 'submit');
  });

  it('disables the button when disabled=true', () => {
    render(Button, { props: { disabled: true, children: () => 'Disabled' } });
    expect(screen.getByRole('button')).toBeDisabled();
  });

  it('calls onclick handler when clicked', async () => {
    const handler = vi.fn();
    render(Button, { props: { onclick: handler, children: () => 'Click' } });
    await fireEvent.click(screen.getByRole('button'));
    expect(handler).toHaveBeenCalledOnce();
  });

  it('does not fire onclick when disabled', async () => {
    const handler = vi.fn();
    render(Button, {
      props: { disabled: true, onclick: handler, children: () => 'No click' },
    });
    await fireEvent.click(screen.getByRole('button'));
    expect(handler).not.toHaveBeenCalled();
  });
});
