// Seam-contract tests for createTreeState.
//
// createTreeState uses $effect (for localStorage persistence) which requires
// a reactive component context. Tests are driven via a thin Svelte harness
// component rendered by @testing-library/svelte, which provides that context.
//
// Contract documented:
//   - initial value defaults to 'tree-collapsed' when localStorage is empty
//   - stored localStorage value is restored on mount
//   - invalid localStorage values fall back to 'tree-collapsed'
//   - cycle() advances the state machine: collapsed → expanded → wide → collapsed
//   - cycle() persists each new state to localStorage so re-mounts restore it

import { describe, it, expect, beforeEach, afterEach } from 'vitest';
import { render, screen, fireEvent, waitFor, cleanup } from '@testing-library/svelte';
import TreeStateHarness from './TreeStateHarness.test.svelte';

describe('createTreeState', () => {
  beforeEach(() => {
    localStorage.clear();
  });

  afterEach(() => {
    cleanup();
  });

  it('initialises to tree-collapsed when localStorage is empty', () => {
    render(TreeStateHarness, { props: { sessionId: 'sess-1' } });
    expect(screen.getByTestId('tree-state-value').textContent).toBe('tree-collapsed');
  });

  it('restores stored tree-expanded value from localStorage', () => {
    localStorage.setItem('jamsesh.tree-state.sess-2', 'tree-expanded');
    render(TreeStateHarness, { props: { sessionId: 'sess-2' } });
    expect(screen.getByTestId('tree-state-value').textContent).toBe('tree-expanded');
  });

  it('ignores invalid localStorage values and defaults to tree-collapsed', () => {
    localStorage.setItem('jamsesh.tree-state.sess-3', 'not-a-real-state');
    render(TreeStateHarness, { props: { sessionId: 'sess-3' } });
    expect(screen.getByTestId('tree-state-value').textContent).toBe('tree-collapsed');
  });

  it('cycle() advances collapsed → expanded → wide → collapsed', async () => {
    render(TreeStateHarness, { props: { sessionId: 'sess-4' } });
    const el = screen.getByTestId('tree-state-value');
    const btn = screen.getByTestId('cycle-btn');

    expect(el.textContent).toBe('tree-collapsed');

    await fireEvent.click(btn);
    await waitFor(() => expect(el.textContent).toBe('tree-expanded'));

    await fireEvent.click(btn);
    await waitFor(() => expect(el.textContent).toBe('tree-wide'));

    await fireEvent.click(btn);
    await waitFor(() => expect(el.textContent).toBe('tree-collapsed'));
  });

  it('cycle() persists to localStorage so a subsequent mount restores the value', async () => {
    render(TreeStateHarness, { props: { sessionId: 'sess-5' } });
    const btn = screen.getByTestId('cycle-btn');

    await fireEvent.click(btn); // collapsed → expanded
    await waitFor(() =>
      expect(localStorage.getItem('jamsesh.tree-state.sess-5')).toBe('tree-expanded'),
    );
  });
});
