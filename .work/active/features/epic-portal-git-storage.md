---
id: epic-portal-git-storage
kind: feature
stage: drafting
tags: [portal]
parent: epic-portal-git
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-16
updated: 2026-05-16
---

# Portal Git — Storage & Lifecycle

## Brief

The on-disk and DB-side layer for session bare repos: storage-path resolution,
bare-repo init/teardown helpers, and the archived-session semantics that
follow the 90-day retention window.

**On-disk layout** (locked at epic-design — no path abstraction layer):

```
<storage>/orgs/<org_id>/sessions/<session_id>.git
```

The storage root is configurable; org and session ids are uuid/ULID strings.
Bare repos use git's standard layout (`HEAD`, `objects/`, `refs/`, etc.).

**Lifecycle:**

- **Create**: called from `POST /api/sessions` (cross-epic call from
  `epic-portal-api`). Atomic with the `sessions` row insert: create the bare
  repo first (`git init --bare`), then commit the session row. On row-insert
  failure, `rm -rf` the half-created repo. Invariant after success: "session
  row exists ⟹ bare repo exists."
- **End** (finalize / abandon / timeout): no repo deletion at end. The repo
  becomes read-only via the pre-receive policy ("session.ended" rejection
  path), retained for the 90-day window so participants can fetch and
  finalize locally.
- **Archive** (90+ days post-end): hard-delete the bare repo directory.
  Insert a row into `archived_sessions` with: `session_id`, `name`, `org_id`,
  `member_account_ids` (string array or JSON), `goal_text`, `ended_at`,
  `end_reason` (`finalize | abandon | timeout`), `final_branch_name`
  (nullable). Delete the original `sessions` row. No restore path by design.
- **Archived stub response**: any HTTP/git request against an archived
  session id returns a 410 Gone with the JSON stub: "This session was
  archived on YYYY-MM-DD. Final branch: `<name>` (pushed to <repo>)." The
  smart-HTTP handler and the REST API both consume the same stub formatter
  from this feature.

**Schema additions** (extending `epic-portal-foundation-data-layer`):
the `archived_sessions` table is owned by this feature (added via a sqlc
migration that this feature ships).

Does NOT cover the smart-HTTP handlers (`smart-http` feature) or
pre/post-receive (their own features). Does NOT cover the retention sweep
trigger — that's a scheduled job that calls into this feature's archive
helper; the trigger itself can be a documented operator cron in v1 or a
deferred internal scheduler.

## Epic context

- Parent epic: `epic-portal-git`
- Position in epic: foundation feature — pre-receive, post-receive, and
  smart-http all consume storage helpers (path resolution, bare-repo
  opening, archived-session lookup).

## Foundation references

- `docs/SPEC.md` — Ref structure, Lifecycle (Creation, End, Retention),
  Deployment shape
- `docs/ARCHITECTURE.md` — Git smart-HTTP component (storage path)
- `docs/SECURITY.md` — Audit trail, What a portal breach exposes

## Inherited epic design decisions

- **Storage path schema**: v1 lock —
  `<storage>/orgs/<org_id>/sessions/<session_id>.git`. No abstraction layer.
- **Archived-session semantics**: hard-delete bare repo + DB rows; retain
  a tiny `archived_sessions` table row for the stub response. No restore.
- **Bare repo init timing**: eager, atomic with session row insert.

<!-- Feature-design will fill in interfaces, signatures, and implementation
units when /agile-workflow:feature-design runs on this. -->
