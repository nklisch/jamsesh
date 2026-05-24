/**
 * useFinalizeLock — wrapper-object rune store for FinalizeView lock state.
 *
 * State machine:
 *   idle → acquiring (POST in flight)
 *   acquiring → held     (POST 200/201 — we own it)
 *   acquiring → conflict (POST 409 — held by another)
 *   acquiring → error    (POST other failure)
 *   held → conflict      (WS session.finalizing + re-acquire → 409)
 *   held → idle          (onDestroy DELETE)
 */
import { client } from '$lib/api/client';
import type { components } from '$lib/api/types.gen';

type LockStatus = components['schemas']['LockStatus'];

// ── Private state ─────────────────────────────────────────────────────────────
let _lock = $state<LockStatus | null>(null);
let _lockConflict = $state<{ holderAccountId: string } | null>(null);
let _lockError = $state<string | null>(null);
let _lockLoading = $state(true);

function _errorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message;
    if (typeof m === 'string' && m.length > 0) return m;
  }
  return fallback;
}

// ── Exported facade ───────────────────────────────────────────────────────────
export const finalizeLock = {
  get status(): LockStatus | null {
    return _lock;
  },
  get conflict(): { holderAccountId: string } | null {
    return _lockConflict;
  },
  get error(): string | null {
    return _lockError;
  },
  get loading(): boolean {
    return _lockLoading;
  },

  dismissError() {
    _lockError = null;
  },

  async acquire(
    orgId: string,
    sessionId: string,
    opts: { override?: boolean } = {},
  ): Promise<void> {
    _lockLoading = true;
    _lockError = null;
    const { data, error, response } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
        body: { override: opts.override === true },
      },
    );
    _lockLoading = false;
    if (error) {
      if (response?.status === 409) {
        const env = error as { details?: Record<string, unknown> };
        const holderId = (env.details?.held_by_account_id as string | undefined) ?? '';
        _lockConflict = { holderAccountId: holderId };
        return;
      }
      _lockError = _errorMessage(error, 'Failed to acquire lock.');
      return;
    }
    if (data) {
      _lock = data;
      _lockConflict = null;
    }
  },

  async release(orgId: string, sessionId: string): Promise<void> {
    if (!_lock) return;
    const lockId = _lock.lock_id;
    // Best-effort fire-and-forget; caller does not await.
    void client.DELETE(
      '/api/orgs/{orgID}/sessions/{sessionID}/finalize/lock/{lockID}',
      { params: { path: { orgID: orgId, sessionID: sessionId, lockID: lockId } } },
    );
  },

  setError(msg: string) {
    _lockError = msg;
  },

  reset() {
    _lock = null;
    _lockConflict = null;
    _lockError = null;
    _lockLoading = true;
  },
};
