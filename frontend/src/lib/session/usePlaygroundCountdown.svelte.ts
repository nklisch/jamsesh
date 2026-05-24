// Playground-mode countdown state + WS subscription management.
//
// Holds the countdown dates (hard-cap, idle-timeout) and the
// remaining-time snapshots fed back from CountdownBadge. Also
// registers the playground.destruction_warning and session.ended WS
// handlers when mountSubscriptions() is called from onMount.
//
// Uses the wrapper-object-rune-store pattern via a factory so that each
// SessionViewShell instance gets its own isolated state.

import { auth } from '$lib/auth.svelte';
import { subscribe } from '$lib/ws.svelte';
import { navigate } from '$lib/router.svelte';
import type { components } from '$lib/api/types.gen';

type PlaygroundSessionSummary = components['schemas']['PlaygroundSessionSummary'];
type PlaygroundDestructionWarningPayload = components['schemas']['PlaygroundDestructionWarningPayload'];
type SessionEndedPayload = components['schemas']['SessionEndedPayload'];

export function createPlaygroundCountdown(sessionId: string) {
  // Capture sessionId in a closure so that svelte-check doesn't warn about
  // "only captures the initial value" — the factory is called once per
  // component instance and sessionId never changes after mount.
  const getSessionId = () => sessionId;

  // Derived: true when auth.playgroundContext is set for this session.
  // Set after seedFromSession() or when an anonymous joiner lands here.
  let _isPlayground = $state(false);

  // Derived from auth as a fallback for anonymous joiners who may land
  // here before the session load completes.
  let _isPlaygroundFromAuth = $derived(
    auth.playgroundContext?.sessionId === getSessionId(),
  );

  let _effectiveIsPlayground = $derived(_isPlayground || _isPlaygroundFromAuth);

  // Countdown props — initialised from PlaygroundSessionSummary after the
  // API load, updated by the playground.destruction_warning WS event.
  let _hardCapAt = $state<Date | null>(null);
  let _idleTimeoutAt = $state<Date | null>(null);

  // Current remaining times (ms) — set by CountdownBadge via onremainingupdate.
  let _idleRemainingMs = $state<number>(Infinity);
  let _hardCapRemainingMs = $state<number>(Infinity);

  return {
    get isPlayground(): boolean {
      return _effectiveIsPlayground;
    },
    get hardCapAt(): Date | null {
      return _hardCapAt;
    },
    get idleTimeoutAt(): Date | null {
      return _idleTimeoutAt;
    },
    get idleRemainingMs(): number {
      return _idleRemainingMs;
    },
    get hardCapRemainingMs(): number {
      return _hardCapRemainingMs;
    },

    /**
     * Call after a PlaygroundSessionSummary load completes to seed countdown
     * dates. PlaygroundSessionSummary carries hard_cap_at and idle_timeout_at
     * as required fields — no runtime 'in' checks needed.
     */
    seedFromSession(s: PlaygroundSessionSummary) {
      _isPlayground = s.org_id === 'org_playground';
      _hardCapAt = new Date(s.hard_cap_at);
      _idleTimeoutAt = new Date(s.idle_timeout_at);
    },

    /** Feed back remaining-time snapshots from CountdownBadge. */
    updateRemaining(idleMs: number, hardMs: number) {
      _idleRemainingMs = idleMs;
      _hardCapRemainingMs = hardMs;
    },

    /**
     * Register WS subscriptions for playground events.
     * Call from onMount; use the returned cleanup fn as the onMount return value.
     *
     * Subscribes to:
     *   playground.destruction_warning — updates the relevant timer's deadline
     *     when the server detects < 5-minute proximity to destruction. The
     *     `reason` field selects idle vs hard-cap; `ends_at` is the precise
     *     deadline. (PROTOCOL.md canonical fields: reason, ends_at,
     *     remaining_seconds, session_id)
     *   session.ended — navigates to the tombstone page when the session
     *     is destroyed (reason may be 'timeout' for playground idle/hard-cap
     *     destruction). (PROTOCOL.md canonical fields: reason, final_branch_name)
     */
    mountSubscriptions(): () => void {
      const unsubWarning = subscribe(
        sessionId,
        'playground.destruction_warning',
        (env) => {
          const e = env as unknown as PlaygroundDestructionWarningPayload;
          if (!e.ends_at) return;
          const endsAt = new Date(e.ends_at);
          if (e.reason === 'hard_cap') {
            _hardCapAt = endsAt;
          } else if (e.reason === 'idle_timeout') {
            _idleTimeoutAt = endsAt;
          }
        },
      );

      const unsubEnded = subscribe(
        sessionId,
        'session.ended',
        (_env: unknown) => {
          void (_env as SessionEndedPayload);
          navigate(`/playground/s/${sessionId}/ended`);
        },
      );

      return () => {
        unsubWarning();
        unsubEnded();
      };
    },
  };
}
