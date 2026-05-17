import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import CommandRunner from './CommandRunner.svelte';

describe('CommandRunner', () => {
  beforeEach(() => {
    vi.useFakeTimers({ shouldAdvanceTime: true });
    Object.defineProperty(navigator, 'clipboard', {
      value: { writeText: vi.fn().mockResolvedValue(undefined) },
      configurable: true,
    });
  });

  afterEach(() => {
    cleanup();
    vi.useRealTimers();
  });

  it('1. not ready: shows preparing hint, no copy button visible', () => {
    render(CommandRunner, { props: { command: '', ready: false } });
    expect(screen.getByText(/preparing command/i)).toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /copy/i })).not.toBeInTheDocument();
    expect(screen.queryByRole('button', { name: /run locally/i })).not.toBeInTheDocument();
  });

  it('2. ready: renders command text and a Copy button', () => {
    const cmd = 'jamsesh finalize-run plan-abc';
    render(CommandRunner, { props: { command: cmd, ready: true } });
    expect(screen.getByText(cmd)).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /copy/i })).toBeInTheDocument();
    expect(screen.getByRole('button', { name: /run locally/i })).toBeInTheDocument();
  });

  it('3. copy button: calls clipboard.writeText with command and shows toast', async () => {
    const cmd = 'jamsesh finalize-run plan-abc';
    render(CommandRunner, { props: { command: cmd, ready: true } });

    const writeText = navigator.clipboard.writeText as ReturnType<typeof vi.fn>;
    await fireEvent.click(screen.getByRole('button', { name: /copy/i }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(cmd);
    });
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(/copied/i);
    });

    // Toast disappears after 1.5s
    await vi.advanceTimersByTimeAsync(1500);
    await waitFor(() => {
      expect(screen.queryByRole('status')).not.toBeInTheDocument();
    });
  });

  it('3b. "Run locally" button also calls clipboard.writeText and shows toast', async () => {
    const cmd = 'jamsesh finalize-run plan-abc';
    render(CommandRunner, { props: { command: cmd, ready: true } });

    const writeText = navigator.clipboard.writeText as ReturnType<typeof vi.fn>;
    await fireEvent.click(screen.getByRole('button', { name: /run locally/i }));

    await waitFor(() => {
      expect(writeText).toHaveBeenCalledWith(cmd);
    });
    await waitFor(() => {
      expect(screen.getByRole('status')).toHaveTextContent(/copied/i);
    });
  });

  it('4. error: errorMessage renders alert, command and buttons hidden', () => {
    render(CommandRunner, {
      props: { command: '', ready: false, errorMessage: 'Failed to fetch plan.' },
    });
    expect(screen.getByRole('alert')).toHaveTextContent('Failed to fetch plan.');
    expect(screen.queryByRole('button', { name: /copy/i })).not.toBeInTheDocument();
    expect(screen.queryByText(/preparing command/i)).not.toBeInTheDocument();
  });

  it('5. disabled: copy and run-locally buttons are disabled', () => {
    const cmd = 'jamsesh finalize-run plan-abc';
    render(CommandRunner, { props: { command: cmd, ready: true, disabled: true } });
    expect(screen.getByRole('button', { name: /copy/i })).toBeDisabled();
    expect(screen.getByRole('button', { name: /run locally/i })).toBeDisabled();
  });

  it('6. oncopy callback fired after successful copy', async () => {
    const onCopy = vi.fn();
    const cmd = 'jamsesh finalize-run plan-xyz';
    render(CommandRunner, { props: { command: cmd, ready: true, oncopy: onCopy } });

    await fireEvent.click(screen.getByRole('button', { name: /copy/i }));

    await waitFor(() => {
      expect(onCopy).toHaveBeenCalledOnce();
    });
  });
});
