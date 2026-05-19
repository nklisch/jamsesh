# Changelog

## v0.1.0

Released 2026-05-18. Initial release.

jamsesh is real-time collaborative AI pair-programming via Claude Code. v0.1.0
ships the full foundation: a Go portal server, a Claude Code plugin, a Svelte
SPA frontend, and supporting deploy/distribution infrastructure.

### Features

- **Portal foundation** — multi-tenant data layer (orgs, accounts, sessions,
  members) with `org_id` enforcement through sqlc-generated queries, TLS-aware
  HTTP skeleton with chi router and shared middleware, OpenAPI-driven REST
  bootstrap, refresh/revoke token flows, GitHub OAuth and magic-link auth with
  pluggable email senders, account/org provisioning, and invite flows.
- **Git smart-HTTP** — `git-upload-pack` / `git-receive-pack` over HTTP with
  bearer auth, bare-repo storage helpers, archive endpoints, pre-receive
  validators (ref naming, size limits, commit metadata, trailer enforcement),
  and post-receive event emission.
- **Auto-merger** — three-way merge engine with go-git, safe-auto-resolve
  semantics for trailer-only conflicts, outcome application back to the bare
  repo, and a worker subscriber driven by the events log.
- **Portal API** — sessions REST (lifecycle, listing, state digest, ref
  actions, invites, member removal), comments REST, events log with OpenAPI
  envelopes, MCP endpoint with `post_comment` / `resolve_comment` / `fork` /
  `query_session_state` tools, and a WebSocket gateway with fanout.
- **Claude Code plugin** — local `jamsesh` binary with browser/device-code
  OAuth, refresh-aware portal client, router state + MCP wiring, fetch/push/
  stop hooks with a retry queue, session slash-commands
  (`join`, `status`, `fork`, `mode`), and packaging with a teaching skill.
- **Portal UI** — Svelte 5 SPA foundation (Vite, routing, login, chrome,
  API/WS token plumbing), design system (tokens, components, fixed test
  fixtures), session list, session view shell with ref tree, artifact pane,
  comment composer, and ref actions (menu, dialogs).
- **Finalize flow** — plan generation (locks schema + REST, fetch token,
  plan fetch + script, mark-shipped semantics), plugin `finalize` /
  `finalize-run` commands with source selection and cleanup, portal curation
  view (screen + route, squash editor, co-author chips).
- **Cloud-native deploy** — routing layer (consistent-hash core, k8s
  discovery, hint cache, MCP header propagation, service wiring, metrics +
  docs), hydration handoff (hydrator, lifecycle, wiring), Postgres-backed
  lease fencing (schema, interface + no-op, Postgres implementation, factory
  + retention), object-storage sync (manifest, pipeline, backend, provider
  extensions, wiring), operational polish (DB pool + lock, `readyz`, metrics,
  secrets-from-file, graceful shutdown, docs).
- **E2E test infrastructure** — module skeleton, Playwright bootstrap,
  Testcontainers fixtures, portal image build, OAuth base-URL configuration,
  CC driver, and CI workflow.
- **E2E test coverage** — golden-path (onboarding/auth, session lifecycle,
  collaborative merge, finalize, CC driver env fix), failure-mode (REST
  validation, config + deps, interrupted ops, SPA error states), chaos
  (network + provider, runtime + clock), and fuzzing (MCP tool input,
  pre-receive validators).
- **E2E CND coverage** — cluster fixture (PortalCluster, MinIO, router image,
  smoke), routing layer (consistent-hash, hint cache, MCP header, k8s
  discovery, 503-retry, backend-dead, pod-disappears chaos), hydration
  handoff (infra, golden, lifecycle, failure, chaos), lease fencing (infra,
  golden, failure, chaos, fuzz), object-storage sync (failure-startup,
  failure-write-rejected, chaos-partition, fuzz-dsn, fuzz-manifest), and
  operational polish (`readyz`, file-secrets, metrics, shutdown deadline,
  migration lock).
- **Distribution** — multi-arch release build pipeline (Linux/macOS/Windows,
  amd64/arm64) with cosign keyless signing, SBOM generation, SLSA build
  provenance, and checksum signing; Docker image build with multi-arch push
  and cosign signing; marketplace publish workflow that mirrors the plugin
  and per-arch binaries; self-host docs (README, SELF_HOST, quickstart CI).

### Security

Gate-security findings hardened before ship:

- Reject GitHub OAuth logins with unverified primary email.
- Validate `fork` ref names against path-traversal in the MCP tool.
- Enforce REST body-size limits and per-route caps.
- Rate-limit auth endpoints (magic-link request, OAuth exchange, refresh).
- Stream `receive-pack` request bodies instead of buffering.
- Require bearer-account match on revoke-token.
- Add security-headers middleware to every response.
- Restrict SQLite default DSN file mode.
- Move WebSocket bearer tokens off the URL onto a ticket exchange and redact
  any residual token-in-URL paths from logs.
- Sanitize HTML rendered from WebSocket events (XSS hardening on
  ActivityFeed).
- Redact tokens in verbose `git` logs and debug logs.
- Shell-escape finalize-script arguments and reject `..` in target branch.
- Move magic-link tokens off the URL into a POST body.
- Authenticate the metrics endpoint.

### Tests

Gate-tests coverage gaps closed before ship — coverage now spans
acceptance-criteria assertions for: ActivityFeed XSS across all event types,
auto-merger apply commit format, safe-resolve skip semantics, finalize lock
concurrent overrides + shell escape, GitHub OAuth unverified email, hint
cache LRU under concurrency, hydration failure unskip, MCP fork ref
traversal, MCP fuzz seed corpus, metrics endpoint auth, object-storage
write-rejected unskip, Postgres lease CI wiring, rate-limit auth,
receive-pack concurrent semaphore, REST body-size cap, revoke-token
cross-account, ring rebalance cardinality, router discovery shutdown, S3
probe failure modes, security headers, stale fencing token unskip, WS bearer
redact, WS client cursor replay fixture.

### Cruft

Gate-cruft cleanups before ship — removed dead code, exported test shims,
and stale compatibility surface: `disableSSL` config, `isPermanentCode`,
`realClock`, `stubs.go`, `timefmt.go`, `withOpenURL`, frontend unused
imports/params, and wired-or-deleted gates for `lifecycle-release`,
`refresher`, `router-kube-discovery`, `automerger-exported-test-shim`,
`buildinfo-string`, `test-only-exports-cluster`.

### Documentation

Gate-docs drift fixes before ship — foundation docs now match
implementation across: architecture (k8s discovery, unscoped routes,
`git-http-backend` removed), protocol (OpenAPI version, WS envelope version,
unscoped routes, missing endpoints), self-host (OAuth callback URL, K8s env
vars, portal URL default, email and OAuth future-release notes), UX
(non-existent slash commands removed), and the `openapi-typescript` repo
skill (version pins refreshed).

### Internal

- 8 reusable code patterns extracted into `.claude/skills/patterns/` with an
  index in `.claude/rules/patterns.md` for future feature work to inherit.
- 4 Low-severity gate findings and 1 product bug deferred to backlog with
  audit trail via `gate_origin` / item bodies.
