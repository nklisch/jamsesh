# Pattern: Testcontainers Fixture Package Shape

Each e2e dependency (Postgres, MailHog, WireMock, Toxiproxy, MinIO,
Portal, portalcluster, Router) is a Go package under
`tests/e2e/fixtures/<name>/` exporting an exported struct (`Postgres`,
`MailHog`, etc.) and a `Start(ctx context.Context, t *testing.T, opts Options) *<Type>`
constructor that returns a ready-to-use fixture with `t.Cleanup`
already registered. The struct carries both the host-side `Host`/`Port`
(for the test process) and a container-side
`ContainerHost`/`ContainerPort` (for other Docker containers — host-port
mappings are not reachable from inside Docker but bridge IPs are).

## Rationale

A consistent constructor shape means tests can compose fixtures without
re-reading each package — `pg := postgres.Start(ctx, t, postgres.Options{})`
then `mh := mailhog.Start(ctx, t)` then `p := portal.Start(ctx, t, portal.Options{DBDSN: pg.ContainerDSN, SMTPHost: mh.ContainerSMTPHost, ...})`.
The dual host/container address fields are required because
Testcontainers' host-mapped ports work from the test process but
inter-container communication must go via the Docker bridge IP.

## Examples

### Example 1: postgres fixture — shared container + per-test DB, with both DSN views

**File**: `tests/e2e/fixtures/postgres/postgres.go:27`

```go
type Postgres struct {
    DSN          string // host-side DSN — for the test process
    ContainerDSN string // bridge-IP DSN — for the portal container
    Host         string
    Port         int
}
```

### Example 2: mailhog fixture — same dual-address shape

**File**: `tests/e2e/fixtures/mailhog/mailhog.go:33`

```go
type MailHog struct {
    SMTPHost          string // host-side
    SMTPPort          int
    ContainerSMTPHost string // bridge IP
    ContainerSMTPPort int    // always 1025
    HTTPURL           string
}
```

### Example 3: portal fixture — same `Start(ctx, t, Options)` shape, consumes other fixtures' Container* fields

**File**: `tests/e2e/fixtures/portal/portal.go:11`

```go
p := portal.Start(ctx, t, portal.Options{
    DBDriver:     "postgres",
    DBDSN:        pg.DSN,
    EmailFrom:    "noreply@example.com",
    SMTPHost:     mh.SMTPHost,
    SMTPPort:     mh.SMTPPort,
    OAuthBaseURL: wm.URL,
})
```

Eight fixture packages (`postgres`, `mailhog`, `portal`, `minio`,
`wiremock`, `portalcluster`, `router`, `toxiproxy`) follow the same
shape, each with a `Start(ctx, t, ...)` constructor.

## When to Use

- Adding a new external dependency to e2e testing (a new Docker-backed
  service that golden/failure tests need).
- The dependency may be consumed from inside another Docker container
  (portal, router, etc.).

## When NOT to Use

- Pure unit tests — use an in-process fake or `sqlitestore` directly.
- One-off scripts that don't need `t.Cleanup` integration — fixtures
  must register cleanup.

## Common Violations

- Returning only a host-side address — the portal fixture cannot dial
  the dep, tests fail with cryptic "connection refused" from the
  container.
- Returning the fixture without registering `t.Cleanup` for container
  teardown — leaks containers between tests.
- Skipping the "image absent → t.Skip with a clear message" guard —
  produces an opaque Docker backtrace on CI nodes missing the image.
