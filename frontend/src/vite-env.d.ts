/// <reference types="svelte" />
/// <reference types="vite/client" />

// Build-time string constants injected by Vite's `define` config block.
// `__APP_VERSION__` is sourced from frontend/package.json at build/test time
// so the colophon does not require a release-coupled literal in source.
// (gate-tests-projectlanding-hardcoded-version-string)
declare const __APP_VERSION__: string;
