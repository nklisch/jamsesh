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
  },
});
