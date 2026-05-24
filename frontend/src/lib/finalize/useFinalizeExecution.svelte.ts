/**
 * useFinalizeExecution — wrapper-object rune store for FinalizeView execution UX.
 *
 * Owns the mark-shipped in-flight flag, session-ended state, and the
 * copied-run-command toast state.
 */
import { client } from '$lib/api/client';

// ── Private state ─────────────────────────────────────────────────────────────
let _copiedRunCommand = $state(false);
let _markShippedInFlight = $state(false);
let _sessionEnded = $state(false);

function _errorMessage(err: unknown, fallback: string): string {
  if (err && typeof err === 'object' && 'message' in err) {
    const m = (err as { message?: unknown }).message;
    if (typeof m === 'string' && m.length > 0) return m;
  }
  return fallback;
}

// ── Exported facade ───────────────────────────────────────────────────────────
export const finalizeExecution = {
  get copiedRunCommand(): boolean {
    return _copiedRunCommand;
  },
  get markShippedInFlight(): boolean {
    return _markShippedInFlight;
  },
  get sessionEnded(): boolean {
    return _sessionEnded;
  },

  markCopied() {
    _copiedRunCommand = true;
  },
  clearCopied() {
    _copiedRunCommand = false;
  },

  endSession() {
    _sessionEnded = true;
  },

  async markShipped(
    orgId: string,
    sessionId: string,
    targetBranch: string,
    onCancelPatch: () => void,
    onError: (msg: string) => void,
  ): Promise<void> {
    if (_markShippedInFlight) return;
    _markShippedInFlight = true;
    // Cancel any pending PATCH; we're about to end the session.
    onCancelPatch();
    const { error } = await client.POST(
      '/api/orgs/{orgID}/sessions/{sessionID}/mark-shipped',
      {
        params: { path: { orgID: orgId, sessionID: sessionId } },
        body: { final_branch_name: targetBranch.trim() || null },
      },
    );
    _markShippedInFlight = false;
    if (error) {
      onError(_errorMessage(error, 'Failed to mark shipped.'));
      return;
    }
    _sessionEnded = true;
  },

  reset() {
    _copiedRunCommand = false;
    _markShippedInFlight = false;
    _sessionEnded = false;
  },
};
