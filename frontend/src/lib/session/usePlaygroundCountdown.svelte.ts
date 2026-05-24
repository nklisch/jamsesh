// Playground-mode countdown state + WS subscription management.
//
// Holds the countdown dates (hard-cap, idle-timeout, last-activity) and
// the remaining-time snapshots fed back from CountdownBadge. Also
// registers the playground.activity_reset and session.destroyed WS
// handlers when mountSubscriptions() is called from onMount.
//
// Uses the wrapper-object-rune-store pattern via a factory so that each
// SessionViewShell instance gets its own isolated state.

import { auth } from '$lib/auth.svelte';
import { subscribe } from '$lib/ws.svelte';
import { navigate } from '$lib/router.svelte';
import type { components } from '$lib/api/types.gen';

type Session = components['schemas']['Session'];

// Playground WS event payload types.
// TODO(idea-playground-ws-event-types-missing-from-openapi): these two event types
// are absent from docs/openapi.yaml and therefore from types.gen.ts. Once the spec
// gap is closed and codegen is re-run, replace these inline annotations with
// generated types from '$lib/api/types.gen' and remove this block.
type PlaygroundActivityResetEvent = {
  type: 'playground.activity_reset';
  last_substantive_activity_at: string; // ISO 8601
  idle_timeout_at: string; // ISO 8601
};
type SessionDestroyedEvent = {
  type: 'session.destroyed';
};

export function createPlaygroundCountdown(sessionId: string) {
  // Capture sessionId in a closure so that svelte-check doesn't warn about
  // "only captures the initial value" — the factory is called once per
  // component instance and sessionId never changes after mount.
  const getSessionId = () => sessionId;

  // Session reference — updated by seedFromSession() after the API load.
  let _session = $state<Session | null>(null);

  // Derived: true when this session belongs to the reserved playground org.
  // Uses auth.playgroundContext as a fallback for anonymous joiners who may
  // land here before the session load completes.
  let _isPlayground = $derived(
    _session?.org_id === 'org_playground' ||
      auth.playgroundContext?.sessionId === getSessionId(),
  );

  // Countdown props — initialised from session data when available, updated by
  // the playground.activity_reset WS event.
  let _hardCapAt = $state<Date | null>(null);
  let _idleTimeoutAt = $state<Date | null>(null);
  let _lastActivityAt = $state<Date | null>(null);

  // Current remaining times (ms) — set by CountdownBadge via onremainingupdate.
  let _idleRemainingMs = $state<number>(Infinity);
  let _hardCapRemainingMs = $state<number>(Infinity);

  return {
    get isPlayground(): boolean {
      return _isPlayground;
    },
    get hardCapAt(): Date | null {
      return _hardCapAt;
    },
    get idleTimeoutAt(): Date | null {
      return _idleTimeoutAt;
    },
    get lastActivityAt(): Date | null {
      return _lastActivityAt;
    },
    get idleRemainingMs(): number {
      return _idleRemainingMs;
    },
    get hardCapRemainingMs(): number {
      return _hardCapRemainingMs;
    },

    /** Call after the session API load completes to seed countdown dates. */
    seedFromSession(s: Session) {
      _session = s;
      if (
        s.org_id === 'org_playground' &&
        'hard_cap_at' in s &&
        'idle_timeout_at' in s &&
        'last_substantive_activity_at' in s
      ) {
        _hardCapAt = new Date(s.hard_cap_at as string);
        _idleTimeoutAt = new Date(s.idle_timeout_at as string);
        _lastActivityAt = new Date(s.last_substantive_activity_at as string);
      }
    },

    /** Feed back remaining-time snapshots from CountdownBadge. */
    updateRemaining(idleMs: number, hardMs: number) {
      _idleRemainingMs = idleMs;
      _hardCapRemainingMs = hardMs;
    },

    /**
     * Register WS subscriptions for playground events.
     * Call from onMount; use the returned cleanup fn as the onMount return value.
     */
    mountSubscriptions(): () => void {
      const unsubActivity = subscribe(
        sessionId,
        'playground.activity_reset',
        (env) => {
          const e = env as unknown as PlaygroundActivityResetEvent;
          if (e.last_substantive_activity_at) {
            _lastActivityAt = new Date(e.last_substantive_activity_at);
          }
          if (e.idle_timeout_at) {
            _idleTimeoutAt = new Date(e.idle_timeout_at);
          }
        },
      );

      const unsubDestroyed = subscribe(
        sessionId,
        'session.destroyed',
        (_env: unknown) => {
          void (_env as SessionDestroyedEvent);
          navigate(`/playground/s/${sessionId}/ended`);
        },
      );

      return () => {
        unsubActivity();
        unsubDestroyed();
      };
    },
  };
}
