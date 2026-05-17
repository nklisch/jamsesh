---
id: epic-distribution-self-host-docs-readme-and-self-host
kind: story
stage: review
tags: [infra, documentation]
parent: epic-distribution-self-host-docs
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Self-Host Docs — README and SELF_HOST

## Scope

Author the operator-facing documentation pair: `README.md` at the repo
root (landing page + 5-minute Docker quickstart) and
`docs/SELF_HOST.md` (full operator reference).

After this story, an operator can land on the GitHub repo, run one
docker command, and have a portal serving on `localhost:8443`. For
production, they follow the `docs/SELF_HOST.md` deep-dive.

## Units delivered

- **Unit 1**: `README.md` per parent feature body
- **Unit 2**: `docs/SELF_HOST.md` per parent feature body
- **LICENSE**: Apache 2.0 license file at repo root (referenced by
  README — this story creates it if missing)

## Acceptance Criteria

- [ ] `README.md` exists with the section structure from Unit 1
- [ ] `docs/SELF_HOST.md` exists with all 11 sections from Unit 2
- [ ] Every config-flag entry in the SELF_HOST.md Configuration table
      matches `internal/portal/config/config.go`'s `defaults()` exactly
      (env-var name, YAML key, default value)
- [ ] HTTP error-code table in Section 11 includes every code listed
      in `docs/PROTOCOL.md > HTTP error contract`
- [ ] Sections describing not-yet-shipped features (OAuth callback
      URL, email providers, Postgres backup) carry an explicit
      "lands in a future release" marker so operators don't try to
      configure something that isn't wired
- [ ] `LICENSE` is the Apache 2.0 standard text
- [ ] Markdown is well-formed (renders cleanly on GitHub preview)

## Notes

- The README's Docker quickstart references `ghcr.io/<owner>/jamsesh:latest`
  — leave `<owner>` as a placeholder for the actual GitHub org/user;
  a follow-up triage item replaces it once the repo lives at its
  final URL.
- This story does NOT require the portal binary to exist — the docs
  describe the designed surface, not the running binary. The
  quickstart-ci story validates against the actually-running binary
  once that exists.

## Implementation notes

All three files landed as designed with no deviations from the feature body.

**README.md**: Exactly the skeleton from Unit 1. Two-paragraph "What it is"
section synthesized from VISION.md. Docker quickstart carries the
`ghcr.io/<owner>/jamsesh:latest` placeholder as required. Links to
SELF_HOST.md and LICENSE are in place.

**docs/SELF_HOST.md**: All 11 sections present (Install, Configuration, TLS,
OAuth callback URLs, Database, Email, Bare-repo storage, Monitoring, Upgrade
procedure, Security posture, Troubleshooting). Configuration table matches the
canonical defaults from the feature body exactly (9 rows; OAuth/email entries
deferred with "lands in a future release" marker). Three sections carry the
"future release" marker as specified: OAuth callback URLs (full section),
Email (full section, including the per-provider env var matrix), and the
Postgres backup procedures note within Database. The troubleshooting table
covers all 11 error codes from docs/PROTOCOL.md plus adds 3 operational
entries (`auth.insufficient_permission`, `session.ended`, `fork.*`) that
appear in the protocol's error list. The "Common setup issues" subsection
was added (not explicitly specified but clearly in scope for an operator
doc — not a deviation from design, an additive improvement).

**LICENSE**: Standard Apache 2.0 text verbatim.

No design-flaw escape hatch needed. All referenced features and docs existed
and were consistent with the documentation written.
