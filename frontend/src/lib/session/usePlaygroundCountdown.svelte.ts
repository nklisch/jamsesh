// Playground-mode countdown state + WS subscription management.
//
// Holds the countdown dates (hard-cap, idle-timeout) and derives the
// remaining-time values (idleRemainingMs, hardCapRemainingMs) from a
// per-instance `now` clock updated by a 1-second setInterval and the Page
// Visibility API. The parent (SessionViewShell) passes these derived values
// directly to CountdownBadge as props — CountdownBadge is display-only and no
// longer pushes per-tick onremainingupdate callbacks into the parent.
//
// Also registers the playground.destruction_warning and session.ended WS
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

  // Per-instance clock — driven by a 1-second setInterval registered in
  // mountSubscriptions(). The interval and visibility listener are tightly
  // coupled to the component lifecycle; they are cleaned up in the return
  // value of mountSubscriptions() (called as the onMount return fn).
  let _now = $state(Date.now());

  // Remaining times derived from the per-instance clock and the deadline dates.
  // These replace the old onremainingupdate child→parent callback.
  let _idleRemainingMs = $derived(
    _idleTimeoutAt ? Math.max(0, _idleTimeoutAt.getTime() - _now) : Infinity,
  );
  let _hardCapRemainingMs = $derived(
    _hardCapAt ? Math.max(0, _hardCapAt.getTime() - _now) : Infinity,
  );

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

    /**
     * Register WS subscriptions and the per-instance clock for playground events.
     * Call from onMount; use the returned cleanup fn as the onMount return value.
     *
     * Registers:
     *   • A 1-second setInterval that advances `_now`, driving the $derived
     *     remaining-time values.
     *   • Page Visibility API handler to snap `_now` to Date.now() when the
     *     tab returns from background, correcting throttle-accumulated drift.
     *   • playground.destruction_warning — updates the relevant timer's deadline.
     *   • session.ended — navigates to the tombstone page.
     */
    mountSubscriptions(): () => void {
      const interval = setInterval(() => {
        _now = Date.now();
      }, 1000);

      function handleVisibility() {
        if (document.visibilityState === 'visible') {
          _now = Date.now();
        }
      }
      document.addEventListener('visibilitychange', handleVisibility);

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
        clearInterval(interval);
        document.removeEventListener('visibilitychange', handleVisibility);
        unsubWarning();
        unsubEnded();
      };
    },
  };
}
