---
id: portalcluster-leaseholder-objid-cast
kind: story
stage: review
tags: [bug, e2e-test, testing]
parent: null
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-17
updated: 2026-05-17
---

# Fix LeaseHolder objid cast: ::bigint vs ::oid

## Problem

`tests/e2e/fixtures/portalcluster/lifecycle.go` `LeaseHolder` queries `pg_locks`
using `AND l.objid = hashtext($1)::bigint`. The portal's own lease test code
(`internal/portal/lease/postgres_test.go`) consistently casts the same value as
`hashtext($1)::oid` (unsigned 32-bit).

`pg_locks.objid` is of type `oid` (unsigned 32-bit). For positive `hashtext`
values the comparison works identically. For negative `hashtext` values (int4
wraps negative) the `::bigint` cast sign-extends to a large positive 64-bit
value which does NOT equal the `oid` value stored in `pg_locks.objid`,
causing `LeaseHolder` to return -1 even when a lease is held.

## Fix

Change the `WHERE` clause in `LeaseHolder` to match the portal's own convention:

```sql
AND l.objid = hashtext($1)::oid
```

## Discovery

Found during code review of `epic-e2e-cnd-coverage-cluster-fixture-portalcluster`
(review 2026-05-17). The `LeaseHolder` helper documents the hashtext portability
risk but the cast mismatch is a separate, narrower bug that may manifest for
any session ID whose `hashtext` value is negative.

## Acceptance

- `LeaseHolder` uses `::oid` cast matching the portal lease code convention
- The smoke test (`epic-e2e-cnd-coverage-cluster-fixture-smoke`) passes
  `LeaseHolder` assertions for at least one acquired session
