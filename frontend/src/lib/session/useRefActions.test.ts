// Seam-contract tests for createRefActions.
//
// Documents the factory's public API contract:
//   - all state starts null / ''
//   - handleRefAction opens the context menu
//   - closeMenu clears the menu
//   - handleMenuAction moves from menu → dialog
//   - closeDialog resets all dialog state
//   - two instances are fully isolated

import { describe, it, expect } from 'vitest';
import { createRefActions } from './useRefActions.svelte';

describe('createRefActions', () => {
  it('starts with no active menu or dialog', () => {
    const ra = createRefActions();
    expect(ra.activeMenuRef).toBeNull();
    expect(ra.activeDialog).toBeNull();
    expect(ra.activeDialogRef).toBe('');
    expect(ra.activeDialogRefMode).toBeUndefined();
  });

  it('handleRefAction stores the menu ref with coordinates', () => {
    const ra = createRefActions();
    ra.handleRefAction({ ref: 'refs/heads/main', action: 'menu', x: 100, y: 200 });
    expect(ra.activeMenuRef).toEqual({ ref: 'refs/heads/main', x: 100, y: 200 });
  });

  it('closeMenu clears activeMenuRef', () => {
    const ra = createRefActions();
    ra.handleRefAction({ ref: 'refs/heads/feat', action: 'menu', x: 50, y: 80 });
    ra.closeMenu();
    expect(ra.activeMenuRef).toBeNull();
  });

  it('handleMenuAction(fork) transitions menu → fork dialog', () => {
    const ra = createRefActions();
    ra.handleRefAction({ ref: 'refs/heads/topic', action: 'menu', x: 0, y: 0 });
    ra.handleMenuAction('fork', 'refs/heads/topic');

    expect(ra.activeMenuRef).toBeNull(); // menu is dismissed
    expect(ra.activeDialog).toBe('fork');
    expect(ra.activeDialogRef).toBe('refs/heads/topic');
  });

  it('handleMenuAction(mode-switch) transitions menu → mode-switch dialog', () => {
    const ra = createRefActions();
    ra.handleRefAction({ ref: 'refs/heads/wip', action: 'menu', x: 0, y: 0 });
    ra.handleMenuAction('mode-switch', 'refs/heads/wip');

    expect(ra.activeDialog).toBe('mode-switch');
    expect(ra.activeDialogRef).toBe('refs/heads/wip');
  });

  it('closeDialog resets dialog to null with empty ref', () => {
    const ra = createRefActions();
    ra.handleRefAction({ ref: 'refs/heads/feat', action: 'menu', x: 0, y: 0 });
    ra.handleMenuAction('fork', 'refs/heads/feat');
    ra.closeDialog();

    expect(ra.activeDialog).toBeNull();
    expect(ra.activeDialogRef).toBe('');
    expect(ra.activeDialogRefMode).toBeUndefined();
  });

  it('two instances are fully isolated', () => {
    const a = createRefActions();
    const b = createRefActions();

    a.handleRefAction({ ref: 'refs/heads/a', action: 'menu', x: 0, y: 0 });
    expect(a.activeMenuRef).not.toBeNull();
    expect(b.activeMenuRef).toBeNull();
  });
});
