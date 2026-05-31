/**
 * useFinalizeCuration — per-instance factory rune store for FinalizeView curation state.
 *
 * Owns the commit selection, ordering, mode, target branch, and commit message.
 * Also owns derived helpers (distinctAuthors, canRun, interactionsDisabled, isCaller).
 * loadRefs is here because refs populate availableGroups (the source pool).
 */
import { client } from '$lib/api/client';
import type { components } from '$lib/api/types.gen';

type PlanMode = components['schemas']['PlanMode'];
type PlanCommit = components['schemas']['PlanCommit'];
type CoAuthor = components['schemas']['CoAuthor'];
type Ref = components['schemas']['Ref'];

// ── Local types ───────────────────────────────────────────────────────────────
export type SourceCommit = {
  sha: string;
  author_name: string;
  author_id: string;
  subject: string;
};
export type RefGroup = {
  ref: string;
  kind: 'draft' | 'isolated' | 'sync';
  tip_sha: string;
  commits: SourceCommit[];
};

// ── Private helpers ───────────────────────────────────────────────────────────

function _shortRefName(ref: string): string {
  return ref.replace(/^refs\/heads\//, '');
}

function _deriveGroupsFromRefs(refs: Ref[], planCommits: PlanCommit[]): RefGroup[] {
  const byRef = new Map<string, RefGroup>();
  const planByPos = new Map(planCommits.map((c) => [c.sha, c]));
  for (const r of refs) {
    const kind: RefGroup['kind'] = r.ref.includes('/draft')
      ? 'draft'
      : r.mode === 'isolated'
        ? 'isolated'
        : 'sync';
    const planMatch = planByPos.get(r.sha);
    const commit: SourceCommit = planMatch
      ? {
          sha: planMatch.sha,
          author_name: planMatch.author_name,
          author_id: planMatch.account_id ?? planMatch.author_email,
          subject: planMatch.subject,
        }
      : {
          sha: r.sha,
          author_name: r.ref.split('/').slice(-2, -1)[0] ?? r.ref,
          author_id: r.ref,
          subject: _shortRefName(r.ref),
        };
    byRef.set(r.ref, { ref: r.ref, kind, tip_sha: r.sha, commits: [commit] });
  }
  return Array.from(byRef.values());
}

// ── Per-instance factory ──────────────────────────────────────────────────────
export function createFinalizeCuration() {
  let _selectedShas = $state<string[]>([]);
  let _availableGroups = $state<RefGroup[]>([]);
  let _mode = $state<PlanMode>('squash');
  let _targetBranch = $state<string>('');
  let _commitMessage = $state<string>('');

  // Derived from plan data (set by useFinalizePlan after a plan load)
  let _distinctAuthors = $state<CoAuthor[]>([]);
  let _isCaller = $state<boolean>(false);

  const _canRun = $derived(
    _selectedShas.length > 0 &&
      _targetBranch.trim().length > 0 &&
      (_mode === 'preserve' || _commitMessage.trim().length > 0),
  );
  const _interactionsDisabled = $derived(
    !_isCaller,
  );

  return {
    get selectedShas(): string[] {
      return _selectedShas;
    },
    get availableGroups(): RefGroup[] {
      return _availableGroups;
    },
    get mode(): PlanMode {
      return _mode;
    },
    get targetBranch(): string {
      return _targetBranch;
    },
    get commitMessage(): string {
      return _commitMessage;
    },
    get distinctAuthors() {
      return _distinctAuthors;
    },
    get isCaller(): boolean {
      return _isCaller;
    },
    get canRun(): boolean {
      return _canRun;
    },
    get interactionsDisabled(): boolean {
      return _interactionsDisabled;
    },

    // ── Setters used by plan module after load ────────────────────────────────
    setDistinctAuthors(authors: CoAuthor[]) {
      _distinctAuthors = authors;
    },
    setIsCaller(val: boolean) {
      _isCaller = val;
    },
    adoptFromPlan(opts: {
      targetBranch?: string;
      commitMessage?: string;
      selectedShas?: string[];
    }) {
      if (opts.targetBranch !== undefined && !_targetBranch) {
        _targetBranch = opts.targetBranch;
      }
      if (
        opts.commitMessage !== undefined &&
        opts.commitMessage !== '' &&
        _commitMessage === ''
      ) {
        _commitMessage = opts.commitMessage;
      }
      if (opts.selectedShas !== undefined && _selectedShas.length === 0) {
        _selectedShas = opts.selectedShas;
      }
    },

    // ── Mutations ─────────────────────────────────────────────────────────────
    addCommit(sha: string) {
      if (_selectedShas.includes(sha)) return;
      _selectedShas = [..._selectedShas, sha];
    },
    removeCommit(sha: string) {
      _selectedShas = _selectedShas.filter((s) => s !== sha);
    },
    moveUp(index: number) {
      if (index <= 0) return;
      const next = [..._selectedShas];
      [next[index - 1], next[index]] = [next[index], next[index - 1]];
      _selectedShas = next;
    },
    moveDown(index: number) {
      if (index >= _selectedShas.length - 1) return;
      const next = [..._selectedShas];
      [next[index + 1], next[index]] = [next[index], next[index + 1]];
      _selectedShas = next;
    },
    addAllInGroup(group: RefGroup) {
      const additions = group.commits
        .map((c) => c.sha)
        .filter((sha) => !_selectedShas.includes(sha));
      if (additions.length === 0) return;
      _selectedShas = [..._selectedShas, ...additions];
    },
    setMode(next: PlanMode) {
      if (_mode === next) return;
      _mode = next;
    },
    setTargetBranch(val: string) {
      _targetBranch = val;
    },
    setCommitMessage(val: string) {
      _commitMessage = val;
    },

    // ── Refs load ─────────────────────────────────────────────────────────────
    async loadRefs(
      orgId: string,
      sessionId: string,
      planCommits: PlanCommit[],
    ): Promise<void> {
      const { data, error } = await client.GET('/api/orgs/{orgID}/sessions/{sessionID}/refs', {
        params: { path: { orgID: orgId, sessionID: sessionId } },
      });
      if (!error && data) {
        _availableGroups = _deriveGroupsFromRefs(data.refs, planCommits);
      }
    },

    reset() {
      _selectedShas = [];
      _availableGroups = [];
      _mode = 'squash';
      _targetBranch = '';
      _commitMessage = '';
      _distinctAuthors = [];
      _isCaller = false;
    },
  };
}
