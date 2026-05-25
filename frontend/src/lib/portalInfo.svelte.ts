// Portal info rune store — deploy-time configuration for anonymous bootstrap.
// Fetches GET /api/portal/info once on init(); caches the result; exposes
// playgroundEnabled and landingVariant. On fetch failure, falls back to
// { playgroundEnabled: false, landingVariant: 'login' } — safe login-bounce
// behaviour rather than an indeterminate landing. Follows the
// wrapper-object-rune-store pattern: Svelte 5 prohibits exporting raw
// $state/$derived from module scope.

import { client } from '$lib/api/client';

// LandingVariant is the string-literal-union for the three deploy modes.
// auto   — redirect anonymous / to /playground if playground is enabled, else /login
// project — render ProjectLanding.svelte in-place at /
// login   — bounce anonymous / to /login (today's default)
export type LandingVariant = 'auto' | 'project' | 'login';

const FALLBACK_PLAYGROUND_ENABLED = false;
const FALLBACK_LANDING_VARIANT: LandingVariant = 'login';

let _playgroundEnabled = $state<boolean>(false);
let _landingVariant = $state<LandingVariant>('login');
let _loaded = $state<boolean>(false);

// Guards a single in-flight fetch. init() is idempotent: subsequent calls
// while loading share the same promise; post-load calls return immediately.
let _fetching: Promise<void> | null = null;

export const portalInfo = {
  get playgroundEnabled(): boolean {
    return _playgroundEnabled;
  },
  get landingVariant(): LandingVariant {
    return _landingVariant;
  },
  /** true after the first fetch completes (success or failure-with-fallback). */
  get loaded(): boolean {
    return _loaded;
  },

  /**
   * Idempotent init. Fetches /api/portal/info once; subsequent calls are
   * no-ops. On network or server failure, falls back to the login-safe
   * defaults and marks loaded=true so the auth gate can proceed.
   */
  init(): Promise<void> {
    if (_loaded) return Promise.resolve();
    if (_fetching !== null) return _fetching;

    _fetching = (async () => {
      try {
        const { data } = await client.GET('/api/portal/info');
        if (data) {
          _playgroundEnabled = data.playground_enabled;
          _landingVariant = data.landing_variant as LandingVariant;
        } else {
          console.warn(
            '[portalInfo] /api/portal/info returned no data — falling back to ' +
              '{ playgroundEnabled: false, landingVariant: "login" }',
          );
          _playgroundEnabled = FALLBACK_PLAYGROUND_ENABLED;
          _landingVariant = FALLBACK_LANDING_VARIANT;
        }
      } catch (err) {
        console.warn(
          '[portalInfo] /api/portal/info fetch failed — falling back to ' +
            '{ playgroundEnabled: false, landingVariant: "login" }. Error:',
          err,
        );
        _playgroundEnabled = FALLBACK_PLAYGROUND_ENABLED;
        _landingVariant = FALLBACK_LANDING_VARIANT;
      } finally {
        _loaded = true;
        _fetching = null;
      }
    })();

    return _fetching;
  },
};
