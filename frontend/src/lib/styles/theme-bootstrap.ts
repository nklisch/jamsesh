// Pre-paint helper — import this as the very first import in app entry
// (before Svelte mount) to set data-theme on <html> before hydration paints.
// Prevents FOUC where the system theme renders then immediately swaps to the
// user's saved preference.
//
// Storage key is 'jamsesh.theme'; valid values: 'light' | 'dark' | 'system'.
// 'system' (and any absent/unknown value) means "respect prefers-color-scheme"
// — no data-theme attribute is set, letting the CSS @media rule take over.

const saved =
  typeof localStorage !== 'undefined'
    ? (localStorage.getItem('jamsesh.theme') as
        | 'system'
        | 'light'
        | 'dark'
        | null)
    : null;

if (saved === 'light' || saved === 'dark') {
  document.documentElement.setAttribute('data-theme', saved);
}
