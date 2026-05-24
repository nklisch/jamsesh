// Seam-contract tests for createNewSessionForm.
//
// Documents the factory's public API contract:
//   - starts with all fields blank, no error, no commands
//   - submit() returns false and sets validationError when orgId is missing
//   - submit() returns true and populates skillCommand/cliCommand on valid input
//   - setGoal / setScopeRaw / setDefaultMode / setInviteesRaw update state
//   - reset() clears output/error state while leaving form fields intact
//   - two instances are fully isolated

import { describe, it, expect } from 'vitest';
import { createNewSessionForm } from './useNewSessionForm.svelte';

describe('createNewSessionForm', () => {
  it('starts with all fields blank and no validation error', () => {
    const f = createNewSessionForm();
    expect(f.goal).toBe('');
    expect(f.scopeRaw).toBe('');
    expect(f.defaultMode).toBe('sync');
    expect(f.inviteesRaw).toBe('');
    expect(f.validationError).toBeNull();
    expect(f.skillCommand).toBe('');
    expect(f.cliCommand).toBe('');
  });

  it('submit() returns false and sets validationError when orgId is blank', () => {
    const f = createNewSessionForm();
    const ok = f.submit('');
    expect(ok).toBe(false);
    expect(f.validationError).toMatch(/no org/i);
  });

  it('submit() returns true and populates both commands on minimal valid input', () => {
    const f = createNewSessionForm();
    f.setGoal('review the auth flow');
    f.setScopeRaw('src/**');
    const ok = f.submit('acme');
    expect(ok).toBe(true);
    expect(f.validationError).toBeNull();
    expect(f.skillCommand).toContain('/jamsesh:jam');
    expect(f.cliCommand).toContain('jamsesh new');
    expect(f.skillCommand).toContain('--org acme');
    expect(f.cliCommand).toContain('--org acme');
  });

  it('submit() includes --invite flag when inviteesRaw is set', () => {
    const f = createNewSessionForm();
    f.setInviteesRaw('alice@example.com,bob@example.com');
    const ok = f.submit('acme');
    expect(ok).toBe(true);
    expect(f.skillCommand).toContain('--invite');
  });

  it('setDefaultMode changes the mode', () => {
    const f = createNewSessionForm();
    f.setDefaultMode('isolated');
    expect(f.defaultMode).toBe('isolated');
  });

  it('reset() clears skillCommand and validationError', () => {
    const f = createNewSessionForm();
    f.setGoal('test');
    f.submit('acme');
    expect(f.skillCommand).not.toBe('');

    f.reset();
    expect(f.skillCommand).toBe('');
    expect(f.cliCommand).toBe('');
    expect(f.validationError).toBeNull();
  });

  it('two instances are fully isolated', () => {
    const a = createNewSessionForm();
    const b = createNewSessionForm();

    a.setGoal('goal-a');
    expect(a.goal).toBe('goal-a');
    expect(b.goal).toBe('');
  });
});
