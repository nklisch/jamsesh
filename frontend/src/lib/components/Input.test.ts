import { describe, it, expect, vi } from 'vitest';
import { render, screen, fireEvent } from '@testing-library/svelte';
import Input from './Input.svelte';

describe('Input', () => {
  it('renders an input element', () => {
    render(Input, {});
    expect(screen.getByRole('textbox')).toBeInTheDocument();
  });

  it('defaults to type=text', () => {
    render(Input, {});
    expect(screen.getByRole('textbox')).toHaveAttribute('type', 'text');
  });

  it('sets type=email when specified', () => {
    render(Input, { props: { type: 'email' } });
    expect(screen.getByRole('textbox')).toHaveAttribute('type', 'email');
  });

  it('renders with placeholder', () => {
    render(Input, { props: { placeholder: 'Enter text' } });
    expect(screen.getByPlaceholderText('Enter text')).toBeInTheDocument();
  });

  it('renders with initial value', () => {
    render(Input, { props: { value: 'hello' } });
    expect(screen.getByRole('textbox')).toHaveValue('hello');
  });

  it('is disabled when disabled=true', () => {
    render(Input, { props: { disabled: true } });
    expect(screen.getByRole('textbox')).toBeDisabled();
  });

  it('calls oninput when value changes', async () => {
    const handler = vi.fn();
    render(Input, { props: { oninput: handler } });
    await fireEvent.input(screen.getByRole('textbox'), {
      target: { value: 'abc' },
    });
    expect(handler).toHaveBeenCalledOnce();
  });

  it('bind:value round-trips', async () => {
    let captured = '';
    const handler = vi.fn((e: Event) => {
      captured = (e.target as HTMLInputElement).value;
    });
    render(Input, { props: { oninput: handler } });
    const input = screen.getByRole('textbox');
    await fireEvent.input(input, { target: { value: 'round-trip' } });
    expect(captured).toBe('round-trip');
  });
});
