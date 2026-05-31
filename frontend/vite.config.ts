import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { svelteTesting } from '@testing-library/svelte/vite';
import { fileURLToPath, URL } from 'url';
import pkg from './package.json' with { type: 'json' };

export default defineConfig({
  plugins: [
    svelte(),
    // svelteTesting injects the `browser` resolve condition during Vitest runs
    // so that Svelte resolves to its client-side build rather than the SSR
    // build. Without this, mount() throws lifecycle_function_unavailable.
    svelteTesting(),
  ],

  // Build-time constants substituted into source at bundle/test time.
  // `__APP_VERSION__` lets ProjectLanding render the canonical version
  // string without a release-coupled literal in source.
  // (gate-tests-projectlanding-hardcoded-version-string)
  define: {
    __APP_VERSION__: JSON.stringify(`v${pkg.version}`),
  },

  resolve: {
    alias: {
      $lib: fileURLToPath(new URL('src/lib', import.meta.url)),
    },
  },

  build: {
    outDir: 'dist',
  },

  server: {
    proxy: {
      '/api': { target: 'http://localhost:8443', changeOrigin: true },
      '/ws':  { target: 'http://localhost:8443', changeOrigin: true, ws: true },
      '/git': { target: 'http://localhost:8443', changeOrigin: true },
      '/mcp': { target: 'http://localhost:8443', changeOrigin: true },
    },
  },

  test: {
    environment: 'jsdom',
    globals: true,
    setupFiles: ['./vitest.setup.ts'],
    // Run workers as worker_threads rather than vitest 2's default `forks`
    // pool. Fork workers are separate child processes; when `vitest run` is
    // hard-killed (terminal close, agent/CI interrupt, OOM-killer) they miss
    // the shutdown IPC, get reparented to init, and spin forever at multi-GB
    // RSS. Threads live inside the runner process and die with it — they
    // cannot orphan that way. Trade-off: marginally weaker isolation than
    // forks, acceptable for this jsdom/Svelte unit suite. `make test-clean`
    // remains as a manual safety net.
    pool: 'threads',
    poolOptions: {
      // Cap concurrency (machine has 16 cores) so a runaway/leaky suite
      // can't exhaust RAM. Workers have been observed at multi-GB RSS; the
      // underlying per-worker memory growth is tracked as a separate bug.
      threads: { minThreads: 1, maxThreads: 8 },
    },
    // Surface per-worker heap usage in the run summary so memory growth is
    // visible instead of climbing silently until the OOM-killer fires.
    logHeapUsage: true,
  },
});
