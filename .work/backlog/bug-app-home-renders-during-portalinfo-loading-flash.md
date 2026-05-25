---
id: bug-app-home-renders-during-portalinfo-loading-flash
created: 2026-05-25
tags: [bug, ui, regression, flash]
---

When `current.name === 'home'`, `auth.isAuthenticated === false`, and `portalInfo.loaded === false`, App.svelte's `{#if}` template falls through to the generic `{:else if current.name === 'home'}` branch and renders `<Home />` (the org picker). The `$effect` at line 51 correctly returns early — holding navigation — but the template has no guard on `portalInfo.loaded` for the generic home branch, so the org-picker flashes briefly before `portalInfo.init()` resolves. The spec (story-portal-visitor-entry-pages-spa-landing, Risks block) promised a transparent loading shell until both `auth.init()` and `portalInfo.init()` resolve; that shell is missing.
