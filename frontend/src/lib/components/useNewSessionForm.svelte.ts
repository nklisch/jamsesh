// ── useNewSessionForm ─────────────────────────────────────────────────────────
//
// Factory that returns a fresh per-drawer rune facade. Each call produces its
// own reactive state so multiple drawer instances do not share state.
//
// Pattern: wrapper-object-rune-store (per-instance factory variant)

/**
 * Shell-escape a value: if it contains any character that would be
 * interpreted by the shell (spaces, quotes, $, !, *, ?, etc.) wrap it
 * in single quotes, escaping any embedded single quotes as '\'' .
 * Plain alphanumeric / dash / dot / slash / star / comma values are
 * returned as-is (common in globs like "src/**" and emails like "a@x.com").
 */
function shellEscape(value: string): string {
  // Characters safe without quoting inside a shell argument:
  //   alphanumeric, -, _, ., /, *, **, comma, @
  if (/^[A-Za-z0-9@._,/*-]+$/.test(value)) {
    return value;
  }
  // Wrap in single quotes; escape embedded single quotes as '\''
  return `'${value.replace(/'/g, "'\\''")}'`;
}

function parseCommaSeparated(raw: string): string[] {
  return raw
    .split(',')
    .map((s) => s.trim())
    .filter(Boolean);
}

/**
 * Build the flag list shared between both command forms.
 * Empty-field omission: if a value is empty after trimming, the
 * corresponding flag is excluded from the rendered command.
 */
function buildFlags(
  org: string,
  goalVal: string,
  scopeVal: string,
  mode: string,
  inviteesVal: string,
): string {
  const flags: string[] = [];

  flags.push(`--org ${shellEscape(org)}`);

  const goalTrimmed = goalVal.trim();
  if (goalTrimmed) {
    flags.push(`--goal ${shellEscape(goalTrimmed)}`);
  }

  const globs = parseCommaSeparated(scopeVal);
  if (globs.length > 0) {
    // Join globs back into a single comma-separated value, then escape as one arg
    flags.push(`--scope ${shellEscape(globs.join(','))}`);
  }

  flags.push(`--mode ${mode}`);

  const invitees = parseCommaSeparated(inviteesVal);
  if (invitees.length > 0) {
    flags.push(`--invite ${shellEscape(invitees.join(','))}`);
  }

  return flags.join(' ');
}

export interface NewSessionFormFacade {
  // Form inputs
  readonly goal: string;
  readonly scopeRaw: string;
  readonly defaultMode: 'sync' | 'isolated';
  readonly inviteesRaw: string;
  readonly validationError: string | null;
  // Output
  readonly skillCommand: string;
  readonly cliCommand: string;
  // Setters (used by bind: workaround via $derived or direct assignment)
  setGoal(v: string): void;
  setScopeRaw(v: string): void;
  setDefaultMode(v: 'sync' | 'isolated'): void;
  setInviteesRaw(v: string): void;
  // Actions
  submit(orgId: string): boolean;
  reset(): void;
}

export function createNewSessionForm(): NewSessionFormFacade {
  let _goal = $state('');
  let _scopeRaw = $state('');
  let _defaultMode = $state<'sync' | 'isolated'>('sync');
  let _inviteesRaw = $state('');
  let _validationError = $state<string | null>(null);
  let _skillCommand = $state('');
  let _cliCommand = $state('');

  return {
    get goal(): string { return _goal; },
    get scopeRaw(): string { return _scopeRaw; },
    get defaultMode(): 'sync' | 'isolated' { return _defaultMode; },
    get inviteesRaw(): string { return _inviteesRaw; },
    get validationError(): string | null { return _validationError; },
    get skillCommand(): string { return _skillCommand; },
    get cliCommand(): string { return _cliCommand; },

    setGoal(v: string): void { _goal = v; },
    setScopeRaw(v: string): void { _scopeRaw = v; },
    setDefaultMode(v: 'sync' | 'isolated'): void { _defaultMode = v; },
    setInviteesRaw(v: string): void { _inviteesRaw = v; },

    /**
     * Validate and compute commands. Returns true if successful (transitions
     * to output view), false if validation failed.
     */
    submit(orgId: string): boolean {
      _validationError = null;

      if (!orgId.trim()) {
        _validationError = 'No org selected.';
        return false;
      }

      // Validate scope globs if provided
      const globs = parseCommaSeparated(_scopeRaw);
      if (_scopeRaw.trim() && globs.length === 0) {
        _validationError = 'Scope must be at least one valid path glob.';
        return false;
      }

      const flags = buildFlags(orgId, _goal, _scopeRaw, _defaultMode, _inviteesRaw);
      _skillCommand = `/jamsesh:jam ${flags}`;
      _cliCommand = `jamsesh new ${flags}`;
      return true;
    },

    reset(): void {
      // Resets transient output/error state for "Edit form" navigation
      _validationError = null;
      _skillCommand = '';
      _cliCommand = '';
    },
  };
}
