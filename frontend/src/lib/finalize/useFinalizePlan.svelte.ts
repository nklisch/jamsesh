/**
 * useFinalizePlan — wrapper-object rune store for FinalizeView plan state.
 *
 * Owns plan fetch and the debounced PATCH on curation changes.
 * Callers supply the lock id and curation values at the time of each
 * PATCH/fetch so the plan module does not import the other rune stores
 * (avoids circular dependency; FinalizeView is the orchestrator).
 */
import { client } from '$lib/api/client';
import type { components } from '$lib/api/types.gen';

type PlanResponse = components['schemas']['PlanResponse'];
type PlanMode = components['schemas']['PlanMode'];

// ── Private state ─────────────────────────────────────────────────────────────
let _plan = $state<PlanResponse | null>(null);
let _planLoading = $state(false);

// Debounce internals — not reactive (identity changes don't drive UI).
let _patchTimer: ReturnType<typeof setTimeout> | null = null;
let _patchSeq = 0;

// ── Exported facade ───────────────────────────────────────────────────────────
export const finalizePlan = {
  get plan(): PlanResponse | null {
    return _plan;
  },
  get loading(): boolean {
    return _planLoading;
  },
  get planId(): string {
    return _plan?.plan_id ?? '';
  },

  /**
   * Fetch the finalize plan for the current lock.
   * Returns the plan on success so callers can derive curation adoptions;
   * returns null on error (sets lockError via the provided callback).
   */
  async refetch(
    orgId: string,
    sessionId: string,
    lockId: string,
    onError: (msg: string) => void,
  ): Promise<PlanResponse | null> {
    _planLoading = true;
    const { data, error } = await client.GET(
      '/api/orgs/{orgID}/sessions/{sessionID}/finalize-plan',
      {
        params: {
          path: { orgID: orgId, sessionID: sessionId },
          query: { lock_id: lockId },
        },
      },
    );
    _planLoading = false;
    if (!error && data) {
      _plan = data;
      return data;
    }
    if (error) {
      onError(_errorMessage(error, 'Failed to fetch plan.'));
    }
    return null;
  },

  /**
   * Schedule a debounced PATCH of the curation state.
   * The caller (FinalizeView) passes the current curation snapshot so
   * flushPatch can send the right values without importing curation store.
   */
  schedulePatch(opts: {
    orgId: string;
    sessionId: string;
    lockId: string;
    selectedShas: string[];
    targetBranch: string;
    baseSha: string;
    mode: PlanMode;
    commitMessage: string;
    onError: (msg: string) => void;
    onSuccess: () => void;
  }) {
    if (_patchTimer !== null) clearTimeout(_patchTimer);
    _patchTimer = setTimeout(() => {
      void _flushPatch(opts);
    }, 300);
  },

  cancelPendingPatch() {
    if (_patchTimer !== null) {
      clearTimeout(_patchTimer);
      _patchTimer = null;
    }
  },

  reset() {
    _plan = null;
    _planLoading = false;
    finalizePlan.cancelPendingPatch();
    _patchSeq = 0;
  },
};

// ── Private helpers ───────────────────────────────────────────────────────────

function _errorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message;
    if (typeof m === 'string' && m.length > 0) return m;
  }
  return fallback;
}

async function _flushPatch(opts: {
  orgId: string;
  sessionId: string;
  lockId: string;
  selectedShas: string[];
  targetBranch: string;
  baseSha: string;
  mode: PlanMode;
  commitMessage: string;
  onError: (msg: string) => void;
  onSuccess: () => void;
}): Promise<void> {
  _patchTimer = null;
  const seq = ++_patchSeq;
  const body = {
    selected_commit_shas: opts.selectedShas,
    target_branch: opts.targetBranch,
    base_sha: opts.baseSha,
    mode: opts.mode,
    commit_message: opts.mode === 'squash' ? opts.commitMessage : null,
  };
  const { error } = await client.PATCH(
    '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}',
    {
      params: { path: { orgID: opts.orgId, sessionID: opts.sessionId, lockID: opts.lockId } },
      body,
    },
  );
  if (seq !== _patchSeq) return; // stale — a newer PATCH has been scheduled/fired
  if (error) {
    opts.onError(_errorMessage(error, 'Failed to update curation state.'));
    return;
  }
  opts.onSuccess();
}
