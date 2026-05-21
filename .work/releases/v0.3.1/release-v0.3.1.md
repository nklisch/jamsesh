---
id: release-v0.3.1
kind: release
stage: released
tags: []
parent: null
depends_on: []
release_binding: v0.3.1
gate_origin: null
created: 2026-05-21
updated: 2026-05-21
---

# Release v0.3.1

Patch release shipping the portal session-attach onboarding feature plus
small follow-ups since v0.3.0. Quality gates skipped per user direction
("ignore normal limitations") — readiness is established by 520/520
frontend tests green + svelte-check clean.

## Bound items

### Feature: portal session-attach onboarding (6)

- `feature-portal-session-attach-onboarding`
  - `story-portal-session-attach-onboarding-walkthrough-component`
  - `story-portal-session-attach-onboarding-help-link`
  - `story-portal-session-attach-onboarding-inviteaccept-integration`
  - `story-portal-session-attach-onboarding-sessionlist-integration`
  - `story-portal-session-attach-onboarding-sessionviewshell-affordance`

## Gate runs

Skipped for this patch.

## Notes

- Plugin SKILL.md refocus (commit `196518c`) shipped without a substrate
  item — picked up automatically because the tag includes all commits.
- `bug-fix` for the drifted bats assertion (`33258ce`) shipped without a
  substrate item; included in the changelog under Internal.

## Shipped (2026-05-21)

**Mapping**: tag-based (annotated `v0.3.1`, pushed to `origin/main`).

**Release commit**: `0ba55f7` (release-prep: v0.3.1)
**Release tag**: `v0.3.1`

**Bound items shipped**: 6 (1 feature + 5 stories)

**Frontend test count**: 520 (unchanged — feature was already drained
and validated before binding).

**Gates skipped per user direction** ("ignore normal limitations").
