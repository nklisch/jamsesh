---
id: e2e-audit-playground-reserved-org-slug-boot-conflict
kind: story
stage: review
tags: [testing, e2e-test, audit, playground]
parent: feature-e2e-playground-coverage-failure
depends_on: []
release_binding: null
gate_origin: null
created: 2026-05-24
updated: 2026-05-24
---

# Reserved `playground` org provisioning at portal boot has no e2e failure test for slug-collision against a pre-existing org row

## Severity
Medium

## Finding type
missing-taxonomy-layer

## Evidence

Unit coverage exists at `internal/portal/playground/provision_test.go`:
- `TestProvisionReservedOrg_NoExistingOrg`
- `TestProvisionReservedOrg_AlreadyProvisioned`
- `TestProvisionReservedOrg_UnprotectedSlugConflict`

The third test name suggests the protection mechanism — the `org_protected`
flag — is asserted at unit scope. Grep for e2e coverage of portal boot
under this condition:

```
$ grep -rIn -E "playground.*org|reserved.*org|org_protected" tests/e2e/
(no output)
```

A related story already exists in active —
`story-gate-tests-reserved-slug-conflict-main-exit-1.md` — covering the
boot-exit behavior. That story focuses on a single boot path; it does
NOT cover the e2e shape (real portal container + real Postgres with a
pre-seeded conflicting org row that lacks `org_protected=true`).

## Why this matters

The `playground` slug is reserved and provisioning runs at portal boot
(per the `playground-activity-reset` pattern in `.claude/rules/patterns.md`
which references `playgroundOrgID`). The failure mode this finding
addresses:

1. A previous portal deploy created an unprotected `playground` org
   (perhaps from a manual operator action or a pre-`org_protected`
   migration path).
2. A new portal version boots and tries to provision the reserved org.
3. Without protection, the unit test verifies the boot path detects this
   and either takes ownership (sets `org_protected=true`) or exits
   non-zero with a recognizable error.
4. The e2e variant verifies real Postgres rows, real boot exit code, and
   real log lines — same shape as
   `tests/e2e/failure/migration_concurrent_startup_test.go`
   ("exactly one container applies the schema").

The existing `story-gate-tests-reserved-slug-conflict-main-exit-1.md`
covers the main.go exit path; this audit finding asks for the full
container-level e2e against a pre-seeded DB.

## Suggested remedy

Add `tests/e2e/failure/playground_reserved_slug_boot_conflict_test.go`
modeled on `migration_concurrent_startup_test.go`'s container-log
inspection pattern. Steps:
1. Start Postgres fixture.
2. Run a SQL fixture that inserts an `orgs` row with slug `playground`
   and `org_protected=false`.
3. Start the portal container with playground enabled.
4. Assert one of two outcomes (depending on the documented contract):
   - Portal exits non-zero AND container log contains a clear error
     mentioning the conflict — OR —
   - Portal boots, takes ownership, and the row now has
     `org_protected=true` (queried directly via Postgres).

Coordinate with `story-gate-tests-reserved-slug-conflict-main-exit-1` so
the two stories assert different layers of the same invariant
(unit + e2e) without duplicating.

## Test sketch

```go
// tests/e2e/failure/playground_reserved_slug_boot_conflict_test.go
func TestPlayground_ReservedSlugBootConflict(t *testing.T) {
    ctx := context.Background()
    pg := postgres.Start(ctx, t, postgres.Options{})

    // Pre-seed an unprotected playground org via direct DB write.
    _, err := pg.Exec(ctx, `
        INSERT INTO orgs (id, slug, name, org_protected)
        VALUES ($1, 'playground', 'Pre-existing', false)
    `, uuid.NewString())
    require.NoError(t, err)

    // Start the portal — playground provisioning runs at boot.
    p, startErr := portal.TryStart(ctx, t, portal.Options{
        DBDriver: "postgres", DBDSN: pg.ContainerDSN,
        PlaygroundEnabled: true,
    })

    if startErr != nil {
        // Outcome A: portal exits non-zero. Assert log mentions conflict.
        logs := p.ContainerLogs(t)
        require.Contains(t, logs, "playground")
        return
    }

    // Outcome B: portal took ownership. Verify row now protected.
    var protected bool
    require.NoError(t, pg.QueryRow(ctx,
        `SELECT org_protected FROM orgs WHERE slug = 'playground'`).Scan(&protected))
    require.True(t, protected, "portal must protect reserved org on boot")
}
```

## Implementation notes

**Outcome implemented by v0.4.0**: Outcome A (exit-on-conflict).
`ProvisionReservedOrg` in `internal/portal/playground/provision.go` returns
`ErrReservedSlugConflict` when it finds an unprotected org with slug
`"playground"`. `cmd/portal/main.go` logs the error (including a remediation
hint) and exits 1. The portal does NOT take ownership of the row.

**No `TryStart` fixture extension needed.** The test uses the existing
`startFailingPortal` helper (defined in `config_and_deps_test.go`) which
starts a container without a health-check wait strategy and polls until it
exits. This helper was already established for boot-failure tests in this
package and is the right surface for Outcome A — no new portal fixture API
was required.

**Setup approach**: boot a migration-only portal (playground disabled) first
so the `orgs` table exists, then pre-seed the conflict row via a direct
`database/sql` connection using `pg.DSN`, then boot the second portal with
playground enabled. This avoids any pre-seeding timing race against an
unschematised database.

**Test assertions**:
1. Container is not running after `startFailingPortal`'s 15-second deadline.
2. Container logs contain at least one of `["reserved slug conflict", "playground"]`.
3. The `org_protected` column on the pre-seeded row is still `false` after
   portal exit — confirming Outcome B (take-ownership) did NOT fire.

**Observed container log output** (from passing test run):
```
{"msg":"playground enabled but reserved slug is taken — refusing to start",
 "err":"reserved slug conflict: an unprotected org with slug \"playground\" exists
        (id=org-preexisting-unprotected); rename it before enabling playground",
 "remediation":"rename the existing 'playground' org or set JAMSESH_PLAYGROUND_ENABLED=false"}
```

**Test runtime**: ~10s (dominated by Postgres container startup and migration
portal boot).
