---
id: epic-distribution-self-host-docs-readme-and-self-host
kind: story
stage: implementing
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
