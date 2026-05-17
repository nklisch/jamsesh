import { defineConfig } from 'vite';
import { svelte } from '@sveltejs/vite-plugin-svelte';
import { svelteTesting } from '@testing-library/svelte/vite';
import { fileURLToPath, URL } from 'url';

export default defineConfig({
  plugins: [
    svelte(),
    // svelteTesting injects the `browser` resolve condition during Vitest runs
    // so that Svelte resolves to its client-side build rather than the SSR
    // build. Without this, mount() throws lifecycle_function_unavailable.
    svelteTesting(),
  ],

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
