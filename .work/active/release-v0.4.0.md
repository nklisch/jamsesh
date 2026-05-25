---
id: release-v0.4.0
kind: release
stage: quality-gate
tags: []
parent: null
depends_on: []
release_binding: v0.4.0
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Release v0.4.0

## Bound items

### Epics (1)

- `epic-ephemeral-playground` — `playground, portal, ui, plugin`

### Features (17)

- `feature-anon-bearer-test-integrity` — `testing, tokens, migrations`
- `feature-epic-ephemeral-playground-anon-bearer` — `portal, security`
- `feature-epic-ephemeral-playground-cli-first-creation` — `plugin, portal`
- `feature-epic-ephemeral-playground-plugin-skills` — `plugin, playground`
- `feature-epic-ephemeral-playground-portal-ui` — `ui, portal, playground`
- `feature-epic-ephemeral-playground-reserved-org` — `portal`
- `feature-epic-ephemeral-playground-session-lifecycle` — `portal, playground`
- `feature-epic-ephemeral-playground-skill-consolidation` — `plugin`
- `feature-playground-foundation-docs-rollup` — `documentation, playground`
- `feature-playground-server-hardening` — `portal, playground`
- `feature-refactor-adapter-dialect-dedup` — `portal, refactor`
- `feature-refactor-adapter-generic-wrap-helpers` — `portal, refactor`
- `feature-refactor-automerger-decomposition` — `portal, refactor`
- `feature-refactor-frontend-god-components` — `ui, refactor`
- `feature-refactor-per-package-clock-compliance` — `portal, refactor, testing`
- `feature-spec-discipline` — `portal, ui, documentation, infra`
- `feature-state-readtoken-per-session-sweep` — `plugin, cleanup, refactor`

### Stories (61)

- `story-adapter-wrap-helpers-step-1-define` — `portal, refactor`
- `story-adapter-wrap-helpers-step-2-sweep` — `portal, refactor`
- `story-anon-bearer-test-integrity-migration-updownup` — `testing, migrations`
- `story-anon-bearer-test-integrity-transactional-rollback` — `testing, tokens`
- `story-cli-invite-dedupe-parseinviteemails-test` — `cleanup, test-debt`
- `story-comments-service-use-slog-not-stdlib-log` — `portal, cleanup, logging`
- `story-epic-ephemeral-playground-cli-first-creation-base-sha` — `portal`
- `story-epic-ephemeral-playground-cli-first-creation-invite` — `plugin`
- `story-epic-ephemeral-playground-cli-first-creation-new` — `plugin`
- `story-epic-ephemeral-playground-plugin-skills-bearer-storage` — `plugin`
- `story-epic-ephemeral-playground-plugin-skills-destruction-warning` — `plugin, playground`
- `story-epic-ephemeral-playground-plugin-skills-jam-consolidation` — `plugin, playground`
- `story-epic-ephemeral-playground-plugin-skills-status-enumeration` — `plugin, playground`
- `story-epic-ephemeral-playground-portal-ui-anonymous-entry` — `ui, playground`
- `story-epic-ephemeral-playground-portal-ui-drawer-rework` — `ui`
- `story-epic-ephemeral-playground-portal-ui-router-refactor` — `ui`
- `story-epic-ephemeral-playground-portal-ui-session-view-extensions` — `ui, playground`
- `story-epic-ephemeral-playground-session-lifecycle-abuse-caps` — `portal, playground, security`
- `story-epic-ephemeral-playground-session-lifecycle-cli-playground-flag` — `plugin, playground`
- `story-epic-ephemeral-playground-session-lifecycle-destruction` — `portal, playground`
- `story-epic-ephemeral-playground-session-lifecycle-docs` — `documentation, playground`
- `story-epic-ephemeral-playground-session-lifecycle-rest-endpoints` — `portal, playground`
- `story-extend-org-protected-guard-to-policy-mutations` — `portal, playground, defense-in-depth`
- `story-foundation-doc-drift-bearer-storage-architecture` — `documentation, plugin`
- `story-orgs-handler-missing-deperr-wrapdb-on-authfail-err` — `portal, security`
- `story-playground-foundation-docs-rollup-architecture-destruction-worker` — `documentation, playground`
- `story-playground-foundation-docs-rollup-protocol-destruction-warning` — `documentation, playground, protocol`
- `story-playground-server-hardening-handler-test-coverage` — `portal, playground, testing`
- `story-playground-server-hardening-wordlist-dedup` — `portal, playground, polish`
- `story-playground-server-hardening-writable-scope-validation` — `portal, playground, validation`
- `story-playground-ws-protocol-mismatch-session-view-extensions` — `bug, playground, portal, ws, protocol`
- `story-refactor-adapter-dialect-dedup-colocate-null-converters` — `portal, refactor`
- `story-refactor-automerger-decomposition-both-modified-helper` — `portal, refactor`
- `story-refactor-automerger-decomposition-flatten-submodule-extract` — `portal, refactor`
- `story-refactor-automerger-decomposition-merge-file-split` — `portal, refactor`
- `story-refactor-automerger-decomposition-side-changes-helper` — `portal, refactor`
- `story-refactor-config-validate-and-env-helpers` — `portal, refactor`
- `story-refactor-events-log-emit-batch-shared-helper` — `portal, refactor`
- `story-refactor-frontend-god-components-finalize-view` — `ui, refactor`
- `story-refactor-frontend-god-components-joiner-picker` — `ui, refactor`
- `story-refactor-frontend-god-components-new-session-drawer` — `ui, refactor`
- `story-refactor-frontend-god-components-org-settings` — `ui, refactor`
- `story-refactor-frontend-god-components-session-attach-walkthrough` — `ui, refactor`
- `story-refactor-frontend-god-components-session-view-shell` — `ui, refactor`
- `story-refactor-per-package-clock-compliance-auth` — `portal, refactor, testing`
- `story-refactor-per-package-clock-compliance-lease` — `portal, refactor, testing`
- `story-refactor-per-package-clock-compliance-objectstore` — `portal, refactor, testing`
- `story-refactor-per-package-clock-compliance-ratelimit` — `portal, refactor, testing`
- `story-refactor-replace-inline-event-types-with-openapi-typescript-gen` — `ui, refactor, cleanup`
- `story-refactor-router-deps-struct-split` — `portal, refactor`
- `story-refactor-view-state-union-comments-and-session-list` — `ui, refactor`
- `story-sessionviewshell-test-playground-branch-coverage` — `test, ui, playground`
- `story-skill-consolidation-primer-stale-slash-refs` — `bug`
- `story-skill-consolidation-references-stale-slash-refs` — `bug, documentation`
- `story-skill-consolidation-rollforward-foundation-docs` — `bug, documentation`
- `story-spec-discipline-audit-and-close-emit-vs-yaml-gaps` — `portal, ui`
- `story-spec-discipline-drift-ci-check` — `portal, infra, testing`
- `story-spec-discipline-pattern-doc` — `documentation`
- `story-state-readtoken-sweep-step-1-helper` — `plugin, refactor`
- `story-state-readtoken-sweep-step-2-callsites` — `plugin, refactor`
- `story-tombstone-expired-redirect-distinguishes-active-vs-expired` — `ui, playground, ux`

## Gate runs

- **gate-security** (2026-05-24) — 9 findings (0 critical, 0 high, 2 medium, 7 low)
  - Medium → active/stories (2): `gate-security-playground-create-handler-no-maxlength-enforcement`, `gate-security-anon-bearer-localstorage-xss-exposure`
  - Low → backlog (7): refresh-error-body-leak, anon-bearer-validate-no-session-binding, githttp-receivepack-wallclock-not-injected, oauth-callback-log-scrubbing, playground-create-orphan-anon-account-on-member-failure, getplaygroundsession-404-vs-401-asymmetry, playground-internal-sql-errors-surface-to-anon
- **gate-tests** (2026-05-24) — 30 findings (2 critical, 9 high, 14 medium, 5 low)
  - Critical → active/stories (2): playground-activity-reset-no-integration-coverage, ratelimit-retryafter-header-unasserted
  - High → active/stories (9): cli-playground-push-failure-recovery, bare-repo-create-orphan-destruction-cleanup, bearer-issuance-tx-split-partial-failure, destruction-concurrent-clustered-race, disable-flip-sweep-still-runs, reserved-slug-conflict-main-exit-1, anon-bearer-cross-session-rejection, magic-link-playground-domain-collision, join-nickname-server-side-validation
  - Medium → active/stories (14): wordlist-empty-or-dashonly-corruption-resistance, hard-cap-idle-timeout-boundary-equality, tombstone-purge-cadence-tick-bound-vs-wallclock, destruction-bearer-revoke-before-cascade-ordering, anon-bearer-display-name-roundtrip-edge-cases, migrate-17-18-up-down-up-isolated, comments-slog-warning-emission-assertion, org-protected-guard-regression-trip, sessionviewshell-hard-cap-reason-branch, tombstone-purge-via-worker-not-store-tautology, frontend-god-components-seam-contracts, applychangesperpath-extracted-branches, state-readtoken-per-session-sweep-callsite-coverage, frontend-set-playground-context-rune-store
  - Low → backlog (5): wordlist-diversity-threshold-and-length-band, event-discriminator-triad-completeness, parseinviteemails-dedupe-location-architectural-note, joinerpicker-410-race-recovery, spec-drift-cwd-resilience
- **gate-cruft** (2026-05-24) — 12 findings (7 high, 3 medium, 2 low)
  - High → active/stories (7): worker-noopLogger-unreachable, handler-test-stepClock-unused, objectstore-countingHydrator-orphaned, objectstore-parsePackedRefsContent-test-only, router-test-unused-beforeEach-import, destructionwarning-test-unused-warn-threshold-const, playground-ratelimit-test-dead-time-second-line
  - Medium → active/stories (3): pushbase-headsha-discard-pattern, pushbasebearer-ctx-placeholder, userpromptsubmit-test-stale-dir-captures
  - Low → backlog (2): oauth-stale-doc-comment-findorprovision, per-package-stores-wrapper-helpers
- **gate-docs** (2026-05-24) — 19 findings (12 high, 7 medium)
  - foundation-doc-assertion (10): spec-playground-sweep-env-var-name-drift, spec-jamsesh-join-slash-command-stale, security-anon-email-separator-drift, protocol-event-types-missing-two, protocol-local-state-schema-block-stale, protocol-rest-route-catalog-missing-playground, protocol-common-error-codes-missing-playground-three, ux-playground-flow-not-documented, architecture-spa-playground-context-not-documented, architecture-security-org-protected-flag-not-documented
  - readme-staleness (2): readme-stale-slash-command-list, readme-playground-mode-not-mentioned
  - pattern-skill-staleness (5): pattern-per-package-clock-package-count-undercount, pattern-dual-dialect-stale-createsession-columns, pattern-openapi-fetch-middleware-stale-anchors, pattern-view-state-union-anchors-and-new-examples, pattern-wrapper-object-rune-store-anchors-and-playgroundcontext
  - changelog-gap (1): changelog-v0-4-0-entry-missing (resolved by release-deploy Phase 5.5)
  - repo-skill-staleness (1): skill-jamsesh-hard-deadlines-prose-brittle
- **gate-patterns** (2026-05-24) — 5 new patterns extracted, 0 inconsistencies
  - New patterns: per-instance-factory-rune-store, adapter-wrap-helpers, strict-server-partial-handler-shim, playground-activity-reset, ticker-sweep-loop
  - Tracking item `gate-patterns-v0.4.0` at stage:done (gate's deliverable is the pattern files themselves)

### Gate re-runs (2026-05-24, post-autopilot drain)

After autopilot drained the original 90+ gate-produced items, all 5 gates re-ran to verify the implementations themselves didn't introduce new issues:

- **gate-security (re-run)** — 0 new findings. The bundle's security posture remains clean across the new implementations (CSP report-uri header, playground maxLength validation, anon-bearer tests, store-partition refactor).
- **gate-tests (re-run)** — 0 new findings. All 25 prior gate-tests items confirmed at stage:done; no new coverage gaps in the autopilot's implementations.
- **gate-cruft (re-run)** — 7 new findings (6 High + 1 Medium), all drained in the same release-deploy cycle:
  - High → active/stories (6): httperr-errinvalidnickname-unused, worker-test-createplaygroundsession-helper-unused, sessions-handler-test-fakeclock-advance-unused, playground-worker-errstopretrying-unused-var, status-compat-typealias-statusoutput-unused, finalize-view-unused-btn-primary-selector
  - Medium → active/stories (1): status-test-stale-setupstatusenv-comment
- **gate-docs (re-run)** — 5 new findings, all drained in the same release-deploy cycle:
  - changelog-foundation-doc-assertion (1): changelog-playground-hardcap-default-wrong (CHANGELOG had `HardCap` 2h; SPEC/code is 24h)
  - pattern-skill-staleness (4): pattern-line-anchors-drift-rerun, pattern-adapter-wrap-helpers-type-name-drift, pattern-per-instance-factory-sessionviewshell-moved, pattern-dual-dialect-occurrence-count-stale
- **gate-patterns (re-run)** — 4 additional patterns extracted (additive, no inconsistencies):
  - package-private-composed-store-interface, test-narrow-store-delegation, testenv-harness-struct, reserved-org-id-local-const-mirror
