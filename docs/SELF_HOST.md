# Self-hosting jamsesh

This is the full operator reference for running a jamsesh portal on your own
infrastructure. If you just want to kick the tires locally, start with the
[README quickstart](../README.md). Come back here when you're ready for TLS,
a real database, OAuth, or production-grade setup.

---

## 1. Install

### Docker (recommended)

The portal ships as a single Docker image. Pull it from the GitHub Container
Registry:

```bash
docker pull ghcr.io/<owner>/jamsesh:latest
```

Image tags follow the release version scheme (`v0.1.0`, `v0.2.0`, etc.).
`latest` tracks the most recent stable release. Pin to a version tag in
production.

**Verify the image signature** (requires [cosign](https://github.com/sigstore/cosign)):

```bash
cosign verify \
  --certificate-identity-regexp 'https://github.com/<owner>/jamsesh/.github/workflows/release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  ghcr.io/<owner>/jamsesh:latest
```

Releases are signed with Sigstore cosign in keyless mode using GitHub OIDC.
The trust anchor is the release workflow identity. See
[docs/SECURITY.md](SECURITY.md) for the full supply-chain model.

### Binary

Download the appropriate binary from the GitHub releases page:

```bash
# Example: Linux amd64
curl -LO https://github.com/<owner>/jamsesh/releases/latest/download/jamsesh-portal-linux-amd64
curl -LO https://github.com/<owner>/jamsesh/releases/latest/download/jamsesh-portal-linux-amd64.sha256
sha256sum -c jamsesh-portal-linux-amd64.sha256
```

**Verify the binary signature**:

```bash
cosign verify-blob \
  --certificate-identity-regexp 'https://github.com/<owner>/jamsesh/.github/workflows/release.yml@refs/heads/main' \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --signature jamsesh-portal-linux-amd64.sig \
  --certificate jamsesh-portal-linux-amd64.pem \
  jamsesh-portal-linux-amd64
```

Install the binary:

```bash
chmod +x jamsesh-portal-linux-amd64
sudo mv jamsesh-portal-linux-amd64 /usr/local/bin/jamsesh-portal
```

**systemd unit example** (`/etc/systemd/system/jamsesh.service`):

```ini
[Unit]
Description=jamsesh portal
After=network.target

[Service]
Type=simple
User=jamsesh
EnvironmentFile=/etc/jamsesh/env
ExecStart=/usr/local/bin/jamsesh-portal
Restart=on-failure
RestartSec=5
; Protect the system from a compromised portal process
NoNewPrivileges=true
PrivateTmp=true
ProtectSystem=strict
ReadWritePaths=/var/lib/jamsesh

[Install]
WantedBy=multi-user.target
```

---

## 2. Configuration

The portal is configured via environment variables or a YAML config file.
Environment variables take precedence over YAML values. Pass the config file
path as the first argument: `jamsesh-portal /etc/jamsesh/config.yaml`.

### Reference table

| Env var | YAML key | Default | Purpose |
|---|---|---|---|
| `JAMSESH_BIND` | `bind` | `:8443` | Listen address (`host:port`) |
| `JAMSESH_DB_DRIVER` | `db_driver` | `sqlite` | Database driver: `sqlite` or `postgres` |
| `JAMSESH_DB_DSN` | `db_dsn` | `./jamsesh.db` | DSN for the selected driver |
| `JAMSESH_TLS_MODE` | `tls.mode` | `behind_proxy` | TLS handling: `native` or `behind_proxy` |
| `JAMSESH_TLS_CERT` | `tls.cert_path` | _(none)_ | Path to TLS certificate — required when `tls.mode=native` |
| `JAMSESH_TLS_KEY` | `tls.key_path` | _(none)_ | Path to TLS private key — required when `tls.mode=native` |
| `JAMSESH_LOG_FORMAT` | `log.format` | `json` | Log output format: `json` or `text` |
| `JAMSESH_LOG_LEVEL` | `log.level` | `0` (Info) | [slog](https://pkg.go.dev/log/slog) level: `-4`=Debug, `0`=Info, `4`=Warn, `8`=Error |
| `JAMSESH_STORAGE` | `storage` | `./storage` | Filesystem path for per-session bare repos |
| `JAMSESH_OAUTH_GITHUB_CLIENT_ID` | `oauth.github.client_id` | _(none)_ | GitHub OAuth application client ID |
| `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET` | `oauth.github.client_secret` | _(none)_ | GitHub OAuth application client secret |
| `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE` | _(env-only)_ | _(none)_ | Path to a file containing the GitHub OAuth client secret; takes precedence over `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET` when set |
| `JAMSESH_OAUTH_GITHUB_BASE_URL` | `oauth.github.base_url` | _(none)_ | Override GitHub OAuth base URL for testing; leave unset in production |
| `JAMSESH_PORTAL_URL` | `portal_url` | _(none)_ | Public base URL of the portal, e.g. `https://jamsesh.example.com`. Required when running behind a reverse proxy that does not forward `Host` and `X-Forwarded-Proto`. Used to construct OAuth callback URLs and magic-link email URLs. |
| `JAMSESH_DB_DSN_FILE` | _(env-only)_ | _(none)_ | Path to a file containing the DB DSN; takes precedence over `JAMSESH_DB_DSN` when set |
| `JAMSESH_DB_MAX_OPEN_CONNS` | `db.max_open_conns` | `25` | Maximum number of open connections in the pool. For Postgres this maps to `pgxpool.MaxConns`; no-op for SQLite (single-writer). |
| `JAMSESH_DB_MAX_IDLE_CONNS` | `db.max_idle_conns` | `5` | Minimum number of idle connections the pool maintains. For Postgres this maps to `pgxpool.MinConns`; no-op for SQLite. |
| `JAMSESH_DB_CONN_MAX_LIFETIME` | `db.conn_max_lifetime` | `30m` | Maximum lifetime of a pooled connection before it is replaced. Go duration string (`30m`, `1h`). For Postgres maps to `pgxpool.MaxConnLifetime`. |
| `JAMSESH_SHUTDOWN_GRACE_S` | `shutdown_grace_s` | `30` | Shared graceful-shutdown budget in seconds. HTTP drain, auto-merger drain, and WebSocket gateway stop all compete within this window. Must be a positive integer. |
| `JAMSESH_EMAIL_SMTP_PASS_FILE` | _(env-only)_ | _(none)_ | Path to a file containing the SMTP password; takes precedence over `JAMSESH_EMAIL_SMTP_PASS` when set |
| `JAMSESH_EMAIL_SENDGRID_API_KEY_FILE` | _(env-only)_ | _(none)_ | Path to a file containing the SendGrid API key; takes precedence over `JAMSESH_EMAIL_SENDGRID_API_KEY` when set |
| `JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE` | _(env-only)_ | _(none)_ | Path to a file containing the Postmark server token; takes precedence over `JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN` when set |
| `JAMSESH_EMAIL_RESEND_API_KEY_FILE` | _(env-only)_ | _(none)_ | Path to a file containing the Resend API key; takes precedence over `JAMSESH_EMAIL_RESEND_API_KEY` when set |
| `JAMSESH_WS_ALLOW_ORIGINS` | _(env-only)_ | _(none)_ | Comma-separated list of allowed `Origin` headers for cross-origin WebSocket upgrades to `/ws/sessions/{sessionID}`. Empty (default) denies all cross-origin upgrades. |

**On `JAMSESH_WS_ALLOW_ORIGINS`.** Same-origin connections (SPA and portal
served from one host) need no entry — leave the var unset. Operators serving
the SPA from a different origin than the portal (a CDN host, a separate
subdomain, a Docker compose dev setup with the SPA on `localhost:5173` and
the portal on `localhost:8443`) must list each accepted public origin
exactly as the browser will send it. Examples:

- `JAMSESH_WS_ALLOW_ORIGINS=https://app.example.com`
- `JAMSESH_WS_ALLOW_ORIGINS=http://localhost:5173,https://app.example.com`

Origins are compared verbatim (scheme + host + port). Wildcards are not
supported. Trailing slashes are not part of an origin and must not be
included.

### `_FILE` convention for secret env vars

Every secret-bearing env var has a `_FILE` companion. When the `_FILE`
variant is set, the portal reads the secret from the file at that path
(trailing whitespace and newlines are trimmed). The `_FILE` variant takes
precedence over the plain env var when both are set. Failure to read a
configured `_FILE` path (missing file, permission denied) is fail-fast at
startup — the portal exits with an error rather than silently using an empty
secret.

The `_FILE` variants are:

| `_FILE` env var | Reads from |
|---|---|
| `JAMSESH_DB_DSN_FILE` | Database DSN |
| `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE` | GitHub OAuth client secret |
| `JAMSESH_EMAIL_SMTP_PASS_FILE` | SMTP password |
| `JAMSESH_EMAIL_SENDGRID_API_KEY_FILE` | SendGrid API key |
| `JAMSESH_EMAIL_POSTMARK_SERVER_TOKEN_FILE` | Postmark server token |
| `JAMSESH_EMAIL_RESEND_API_KEY_FILE` | Resend API key |

This convention integrates naturally with:

- **Kubernetes Secrets** mounted as files (set `JAMSESH_DB_DSN_FILE=/run/secrets/db-dsn`)
- **Docker Swarm secrets** (secrets are mounted at `/run/secrets/<name>` by default)
- **Google Secret Manager** / **AWS Secrets Manager** with a sidecar that writes secret
  values to a volume (e.g., `secrets-store-csi-driver`)
- Any secrets manager that can write a file — the portal reads it once at startup

Non-secret env vars (`JAMSESH_BIND`, `JAMSESH_PORTAL_URL`, `JAMSESH_LOG_LEVEL`, etc.)
do not have `_FILE` variants; they use plain env vars only.

### Example YAML config file

```yaml
bind: ":8443"
tls:
  mode: native
  cert_path: /etc/ssl/jamsesh/fullchain.pem
  key_path: /etc/ssl/jamsesh/privkey.pem

db_driver: sqlite
db_dsn: /var/lib/jamsesh/jamsesh.db

storage: /var/lib/jamsesh/storage

log:
  format: json
  level: 0
```

---

## 3. TLS

### `behind_proxy` mode (recommended for most setups)

The portal speaks plain HTTP and trusts a TLS-terminating reverse proxy in
front of it. This is the default (`JAMSESH_TLS_MODE=behind_proxy`). Your
reverse proxy handles certificates, HSTS headers, and client connection
management.

**Caddy** (`/etc/caddy/Caddyfile`):

```
jamsesh.example.com {
    reverse_proxy localhost:8443
}
```

Caddy obtains and renews Let's Encrypt certificates automatically.

**nginx** (`/etc/nginx/sites-available/jamsesh`):

```nginx
server {
    listen 443 ssl http2;
    server_name jamsesh.example.com;

    ssl_certificate     /etc/letsencrypt/live/jamsesh.example.com/fullchain.pem;
    ssl_certificate_key /etc/letsencrypt/live/jamsesh.example.com/privkey.pem;

    location / {
        proxy_pass http://127.0.0.1:8443;
        proxy_set_header Host $host;
        proxy_set_header X-Real-IP $remote_addr;
        proxy_set_header X-Forwarded-For $proxy_add_x_forwarded_for;
        proxy_set_header X-Forwarded-Proto $scheme;

        # WebSocket support
        proxy_http_version 1.1;
        proxy_set_header Upgrade $http_upgrade;
        proxy_set_header Connection "upgrade";
        proxy_read_timeout 3600s;
    }
}
```

### `native` mode

The portal terminates TLS itself. Set `JAMSESH_TLS_MODE=native` and supply
cert and key paths:

```bash
JAMSESH_TLS_MODE=native \
JAMSESH_TLS_CERT=/etc/ssl/jamsesh/fullchain.pem \
JAMSESH_TLS_KEY=/etc/ssl/jamsesh/privkey.pem \
jamsesh-portal
```

For Let's Encrypt in native mode, obtain a certificate with `certbot certonly`
(standalone or webroot), then point the portal at the resulting files. Note
that certificate renewal requires a portal restart or a SIGHUP if you add
reload support; using Caddy in `behind_proxy` mode sidesteps this entirely.

---

## 4. OAuth callback URLs

> **NOTE:** OAuth provider configuration lands with
> `epic-portal-foundation-auth-flows` in a future release. This section
> describes the expected callback shape based on the auth-flows feature design;
> the exact env vars and registration steps will be filled in when that feature
> ships.

**GitHub OAuth** (the initial supported provider):

1. Go to **GitHub → Settings → Developer settings → OAuth Apps → New OAuth App**.
2. Set **Authorization callback URL** to:
   ```
   https://<your-portal-host>/auth/github/callback
   ```
3. Copy the **Client ID** and **Client Secret** and set them in the portal
   config (env vars for the OAuth provider — see the auth-flows release notes
   for the exact variable names).

The portal discovers its own callback URL from its configured bind address and
TLS mode. If you're behind a reverse proxy, ensure `X-Forwarded-Proto` and
`Host` headers are forwarded correctly (both Caddy and nginx examples above
do this).

Google and OIDC provider support lands in a future release.

---

## 5. Database

### SQLite (default)

SQLite is the default and requires zero additional infrastructure. It works
well for single-node self-host deployments with moderate session volume.

The database file is created automatically at first startup. Back it up by
stopping the portal and copying the file, or by using SQLite's online backup:

```bash
sqlite3 /var/lib/jamsesh/jamsesh.db ".backup /var/backups/jamsesh-$(date +%Y%m%d).db"
```

For automated backups, run this as a cron job. The file is small — a few
megabytes for typical usage — and backs up quickly.

### Postgres

Set `JAMSESH_DB_DRIVER=postgres` and provide a Postgres DSN:

```bash
JAMSESH_DB_DRIVER=postgres \
JAMSESH_DB_DSN="host=localhost user=jamsesh password=<pw> dbname=jamsesh sslmode=require" \
jamsesh-portal
```

Postgres is the right choice when you need horizontal scaling or want to use
your existing database backup infrastructure. The schema is identical to
SQLite; the portal selects the appropriate sqlc-generated query package at
runtime. Postgres-specific backup procedures (pg_dump, WAL archiving) are
standard practice — detailed recipes for managed Postgres services land in a
future release.

### Migrations

Database migrations are generated by [sqlc](https://sqlc.dev/) and ship as
SQL files alongside each release. Migrations run automatically on portal
startup and are idempotent — restarting the portal after an upgrade is safe.
No manual migration step is required.

---

## 6. Email (magic-link delivery)

> **NOTE:** Email provider configuration lands with
> `epic-portal-foundation-auth-flows` in a future release. This section
> describes the expected provider options.

The portal supports magic-link email auth as an alternative to OAuth for
headless or browser-free environments. Provider options will include SMTP
(self-host default), SendGrid, Postmark, and Resend. Per-provider env var
configuration will be documented in the auth-flows release notes.

---

## 7. Bare-repo storage

Each session gets a bare git repository on disk under the configured `storage`
directory:

```
<storage>/
└── orgs/
    └── <org-id>/
        └── sessions/
            └── <session-id>.git/
```

**Disk usage**: expect roughly 20–50 MB per active session in typical
doc-writing or spec jams, depending on commit frequency and file sizes. Code
jams with large generated files can be larger.

**Backup**: back up the entire `storage/` directory. It is plain git data —
any filesystem backup tool works. Stop the portal first for a clean snapshot,
or use a live filesystem snapshot (LVM, ZFS) to avoid backing up a repo in
the middle of a receive-pack. The bare repos are self-contained; restoring the
directory restores all session content.

**Retention**: sessions are readable for 90 days after they end (per the
project spec). After that, the bare repo and associated social state are
archived. Archived sessions return a summary stub from the portal API.

---

## 8. Monitoring

### Log output

The portal emits structured logs to stdout. With `JAMSESH_LOG_FORMAT=json`
(the default), each line is a JSON object compatible with most log ingestion
pipelines (Datadog, Loki, CloudWatch Logs, etc.):

```json
{"time":"2026-01-15T10:23:45Z","level":"INFO","msg":"push received","session_id":"abc123","user_id":"u_xyz","ref":"jam/abc123/alice/main","commit_count":1}
```

With `JAMSESH_LOG_FORMAT=text`, logs are human-readable (useful for local
development):

```
2026-01-15 10:23:45 INFO push received session_id=abc123 user_id=u_xyz ref=jam/abc123/alice/main
```

Useful log attributes to watch:

| Attribute | Meaning |
|---|---|
| `session_id` | which session the event belongs to |
| `user_id` | which account triggered the event |
| `ref` | the git ref being operated on |
| `commit_count` | commits in a push |
| `merge_result` | `succeeded` or `conflict` on auto-merger events |
| `error` | machine-readable error code on failures |

### Metrics

The portal exposes a Prometheus metrics endpoint at `/metrics` in the
standard [text exposition format](https://prometheus.io/docs/instrumenting/exposition_formats/).
No authentication is required. Point your Prometheus scrape config at the
portal's host and port:

```yaml
scrape_configs:
  - job_name: jamsesh
    static_configs:
      - targets: ["jamsesh.example.com:8443"]
    scheme: https
```

**Exposed metrics:**

| Metric | Type | Labels | Description |
|---|---|---|---|
| `http_requests_total` | counter | `method`, `route`, `status` | Total HTTP requests. Route labels are chi route patterns (`/api/orgs/{orgID}/sessions/{sessionID}`), not raw URLs — cardinality is bounded by the route table. |
| `http_request_duration_seconds` | histogram | `method`, `route` | Request latency. Buckets span 5ms–10s (exponential). |
| `jamsesh_git_pushes_total` | counter | `result` | Git pushes. `result` is `ok` or `rejected`. |
| `jamsesh_automerger_outcomes_total` | counter | `outcome` | Auto-merger attempts. `outcome` is `succeeded`, `conflict`, or `backpressure`. |
| `jamsesh_event_log_emit_total` | counter | _(none)_ | Event-log entries committed to the database. |
| `go_goroutines`, `go_memstats_*`, `process_cpu_seconds_total`, … | — | — | Standard Go runtime and process metrics, collected automatically. |

Key signals to alert on:

- `rate(jamsesh_git_pushes_total{result="rejected"}[5m])` — elevated rejection rate
  indicates auth or scope problems.
- `rate(jamsesh_automerger_outcomes_total{outcome="conflict"}[5m])` — high conflict
  rate may indicate misaligned session scopes.
- `rate(http_requests_total{status=~"5.."}[5m])` — server-error rate.

### Readiness probe

`GET /readyz` returns the portal's readiness as a JSON object. Use it for
container readiness probes, load-balancer health checks, and CI deployment
gates.

**200 OK — ready:**

```json
{
  "status": "ready",
  "checks": [
    {"name": "db",      "ok": true},
    {"name": "storage", "ok": true}
  ]
}
```

**503 Service Unavailable — not ready:**

```json
{
  "status": "not_ready",
  "checks": [
    {"name": "db",      "ok": false, "error": "dial tcp: connection refused"},
    {"name": "storage", "ok": true}
  ]
}
```

Each probe runs in parallel with a 2-second per-check timeout. A check that
exceeds 2s reports `"error": "timeout"`. The default probe set:

| Check | What it tests |
|---|---|
| `db` | `Ping` against the configured database (SQLite or Postgres) |
| `storage` | `stat` on the configured storage root directory |

`GET /healthz` is a liveness check that returns `{"status":"ok"}` as long as
the process is running. Use `/healthz` for liveness probes and `/readyz` for
readiness probes — they serve different purposes and should not be conflated.

### Log-based alerting

The JSON log stream is a complementary observability surface. Key log
attributes for alert routing:

- Error rate on `push received` events (indicates auth or scope problems).
- `merge_result=conflict` rate (high rate may indicate misaligned session scopes).
- Portal crash / restart events (systemd or container restarts).

---

## 9. Upgrade procedure

### Docker

```bash
docker pull ghcr.io/<owner>/jamsesh:<new-version>
docker stop jamsesh
docker rm jamsesh
docker run --rm -d --name jamsesh \
  -p 8443:8443 \
  -e JAMSESH_TLS_MODE=behind_proxy \
  -v /var/lib/jamsesh:/data \
  ghcr.io/<owner>/jamsesh:<new-version>
```

The portal runs database migrations automatically on startup. If the upgrade
includes schema changes, the migration runs before the portal accepts
connections.

### Binary / systemd

```bash
# Download and verify the new binary (see Install section)
sudo systemctl stop jamsesh
sudo mv /usr/local/bin/jamsesh-portal /usr/local/bin/jamsesh-portal.bak
sudo mv jamsesh-portal-linux-amd64 /usr/local/bin/jamsesh-portal
sudo chmod +x /usr/local/bin/jamsesh-portal
sudo systemctl start jamsesh
```

### Graceful shutdown

On SIGTERM the portal drains in-flight requests, the auto-merger queue, and
the WebSocket gateway before exiting. All drain steps share a single wall-clock
budget controlled by `JAMSESH_SHUTDOWN_GRACE_S` (default `30`). The portal
logs per-step elapsed times at shutdown:

```
{"level":"INFO","msg":"shutdown complete","shutdown_step":"http","elapsed_ms":180}
{"level":"INFO","msg":"shutdown complete","shutdown_step":"automerger","elapsed_ms":45}
{"level":"INFO","msg":"shutdown complete","shutdown_step":"wsgateway","elapsed_ms":12}
```

For Kubernetes deployments, set `terminationGracePeriodSeconds` to
`JAMSESH_SHUTDOWN_GRACE_S + 5` to give the portal time to finish draining
before the kubelet sends SIGKILL (see the k8s recipe in §13).

### When migrations are required

Every release notes document lists any schema migrations included. All
migrations are backward-safe (additive: new columns, new tables). There are
no destructive migrations in v1 — the portal can always start against a
database from the previous release.

When using Postgres, concurrent pod restarts during a rolling deploy are safe:
the portal serializes migration execution with a Postgres advisory lock
(`pg_advisory_lock(8675309)`). Only one pod runs migrations at a time; the
others wait and then skip the already-applied migrations. The lock releases
automatically if the migrating process dies mid-run.

### Rollback

If a release has a critical bug, stop the portal, restore the pre-upgrade
binary backup, and restart. If the release included a migration, you'll need
to restore a pre-upgrade database backup — migrations do not self-revert.
This is why taking a database backup immediately before upgrading is
recommended.

---

## 10. Security posture

The short version: TLS termination is your job (in `behind_proxy` mode),
and keeping the portal binary up to date is your job. Everything else is
handled by the portal.

**Operator responsibilities** (from [docs/SECURITY.md](SECURITY.md)):

- TLS termination (recommended via reverse proxy with Let's Encrypt or similar)
- Database backup and disaster recovery
- Network access controls — firewall rules for who can reach the portal
- OAuth callback URL configuration
- Patching the portal binary as security updates ship

**What the portal handles**:

- All client-to-portal communication is HTTPS (or proxied HTTPS).
- OAuth tokens are stored hashed at rest; refresh tokens are scoped.
- Token revocation propagates within 1 minute — active sessions verify on
  every protected request.
- Every push is validated server-side by `pre-receive`. A buggy or adversarial
  plugin cannot push to another user's refs or outside the session's writable
  scope.
- The portal makes no API calls to external forges and holds no source-repo
  credentials.

**Breach impact**: in the worst case (full database + filesystem read), an
attacker gets session content, comments, and OAuth refresh tokens — but no
source-repo credentials and no data from sessions outside the portal's scope.
See [docs/SECURITY.md](SECURITY.md) for the complete breach-impact analysis
and the full supply-chain and integrity model.

---

## 11. Troubleshooting

### Health check

```bash
curl http://localhost:8443/healthz
# → {"status":"ok"}
```

If the portal isn't responding, check that it started without errors
(`journalctl -u jamsesh -n 50` for systemd, `docker logs jamsesh` for
Docker).

### HTTP error codes

Portal API errors return JSON with a machine-readable `error` code:

```json
{"error":"auth.invalid_token","message":"token signature invalid","details":{}}
```

Common codes and what they mean operationally:

| Error code | Likely cause | Operator action |
|---|---|---|
| `auth.invalid_token` | Malformed or tampered token | Check for client clock skew > 5 minutes; verify OAuth configuration is correct |
| `auth.expired_token` | Access token expired and refresh is failing | Check that the portal's OAuth client credentials are valid; verify the OAuth provider is reachable |
| `auth.insufficient_permission` | Role mismatch on an admin endpoint | Normal for non-admin users hitting admin routes; review your user's org role |
| `session.not_found` | Session ID doesn't exist or is archived | Verify the session ID; check if the session has passed its 90-day retention window |
| `session.not_member` | User is not a member of the session | Invite the user to the session via the portal UI |
| `session.ended` | Session has been finalized or abandoned | No further writes; reads are available until archival |
| `push.scope_violation` | Commit touches paths outside the session's writable scope | The `details.paths` field lists the offending paths; review the session scope or the commit's changes |
| `push.ref_namespace_violation` | Push targets a ref outside the user's namespace | Indicates a client misconfiguration; the plugin should not be generating pushes outside `jam/<session>/<user>/*` |
| `push.missing_trailer` | Commit is missing required trailers | Client plugin version mismatch — the `jamsesh` plugin is not injecting required trailers; upgrade the plugin |
| `fork.target_not_found` | Specified commit SHA doesn't exist in the session repo | Verify the commit exists with `git fetch`; the repo may need a fetch |
| `fork.invalid_target_ref` | Target ref name is invalid or outside allowed namespace | Review the ref name; it must be under `jam/<session>/<user>/` |

See [docs/PROTOCOL.md](PROTOCOL.md) for the full HTTP error contract and the
complete list of error codes.

---

## 12. CI

The `e2e` GitHub Actions workflow runs the full end-to-end suite on every PR and push to main — see `.github/workflows/e2e.yml`. It is the canonical e2e gate: Go fixture tests (Testcontainers-Go), ccdriver integration, and Playwright browser tests all run in a single `make test-e2e` invocation. Playwright traces are uploaded as artifacts on failure for debugging.

---

## 13. Cloud deploy recipes

The portal is a single binary that binds on `JAMSESH_BIND` (default `:8443`)
and persists state to a database and a storage directory. Cloud deploys need
to supply both. The recipes below are concrete starting points; adapt secrets
management and resource sizing to your environment.

### Google Cloud Run

Cloud Run terminates TLS and handles HTTPS routing automatically. The portal
runs in `behind_proxy` mode (the default). Cloud Run's request timeout caps at
60 minutes — WebSocket sessions that run longer will be cut by the platform.
For interactive jam sessions this is acceptable; set `min-instances=1` to keep
a warm instance and avoid cold-start latency mid-session.

Cloud Run's instance filesystem is ephemeral. For any non-trivial deployment:
- Use **Cloud SQL (Postgres)** for the database; connect via the Cloud SQL Auth Proxy
  socket injected by `--add-cloudsql-instances`.
- Use Cloud SQL's managed storage; the portal's `storage` directory (bare repos)
  must also be persisted — Cloud Run doesn't offer a persistent volume, so
  either use a mounted GCS FUSE sidecar or switch to a VM-based deploy for
  bare-repo persistence. Single-instance Cloud Run with a persistent disk
  add-on (preview feature) is an alternative.

**Store the OAuth secret in Secret Manager and mount it as a file:**

```bash
# Create the secret
echo -n "your-github-oauth-secret" | \
  gcloud secrets create jamsesh-github-secret --data-file=-

# Deploy
gcloud run deploy jamsesh \
  --image ghcr.io/<owner>/jamsesh:v0.2.0 \
  --region us-central1 \
  --port 8443 \
  --min-instances 1 \
  --timeout 3600 \
  --set-env-vars "JAMSESH_TLS_MODE=behind_proxy" \
  --set-env-vars "JAMSESH_DB_DRIVER=postgres" \
  --set-env-vars "JAMSESH_DB_DSN=host=/cloudsql/PROJECT:REGION:INSTANCE user=jamsesh dbname=jamsesh sslmode=disable" \
  --set-env-vars "JAMSESH_OAUTH_GITHUB_CLIENT_ID=your-client-id" \
  --set-env-vars "JAMSESH_PORTAL_URL=https://jamsesh-<hash>-uc.a.run.app" \
  --set-env-vars "JAMSESH_STORAGE=/tmp/storage" \
  --set-env-vars "JAMSESH_WS_ALLOW_ORIGINS=https://your-spa-domain.example.com" \
  --set-secrets "JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE=jamsesh-github-secret:latest" \
  --add-cloudsql-instances PROJECT:REGION:INSTANCE \
  --service-account jamsesh-sa@PROJECT.iam.gserviceaccount.com
```

`--set-secrets JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE=...` mounts the
Secret Manager secret as a file and sets the env var to its path — exactly
what the `_FILE` convention expects. The portal reads and trims the file
at startup.

Set `JAMSESH_WS_ALLOW_ORIGINS` if your SPA is served from a domain different
from the Cloud Run service URL (common when the SPA is on a CDN).

### Fly.io

Fly.io provides persistent volumes, which makes it a natural fit for the
portal's bare-repo storage. The deploy strategy `immediate` replaces the
running instance in-place rather than doing a rolling deploy — correct for a
single-instance stateful service.

**`fly.toml`:**

```toml
app = "jamsesh"
primary_region = "ord"

[build]
  image = "ghcr.io/<owner>/jamsesh:v0.2.0"

[deploy]
  strategy = "immediate"

[http_service]
  internal_port = 8443
  force_https = true
  [[http_service.checks]]
    path = "/healthz"
    interval = "10s"
    timeout = "5s"
  [[http_service.checks]]
    path = "/readyz"
    interval = "15s"
    timeout = "5s"
    grace_period = "15s"

kill_signal = "SIGTERM"
kill_timeout = "35s"

[[mounts]]
  source = "jamsesh_data"
  destination = "/data"

[env]
  JAMSESH_BIND            = ":8443"
  JAMSESH_TLS_MODE        = "behind_proxy"
  JAMSESH_DB_DRIVER       = "sqlite"
  JAMSESH_DB_DSN          = "/data/jamsesh.db"
  JAMSESH_STORAGE         = "/data/storage"
  JAMSESH_PORTAL_URL      = "https://jamsesh.fly.dev"
  JAMSESH_SHUTDOWN_GRACE_S = "30"
```

**Create the volume and set secrets:**

```bash
fly volumes create jamsesh_data --region ord --size 10
fly secrets set \
  JAMSESH_OAUTH_GITHUB_CLIENT_ID=your-client-id \
  JAMSESH_OAUTH_GITHUB_CLIENT_SECRET=your-client-secret
fly deploy
```

Fly secrets are injected as env vars in the container environment (not file
mounts), so use the plain `JAMSESH_OAUTH_GITHUB_CLIENT_SECRET` form rather
than the `_FILE` variant.

`kill_timeout = "35s"` gives the portal its full 30-second grace window plus a
5-second buffer before Fly sends SIGKILL.

For Postgres, add a Fly Postgres cluster and set:
```bash
fly secrets set JAMSESH_DB_DSN="postgres://user:pass@top2.nearest.of.jamsesh-db.internal/jamsesh"
```

### Railway

Railway auto-detects the Dockerfile (or uses the image directly). The
simplest deploy uses Railway's Postgres plugin, which injects `DATABASE_URL`
into the environment.

**`railway.json`:**

```json
{
  "$schema": "https://railway.app/railway.schema.json",
  "build": {
    "builder": "DOCKERFILE"
  },
  "deploy": {
    "startCommand": "/jamsesh-portal",
    "healthcheckPath": "/healthz",
    "healthcheckTimeout": 10,
    "restartPolicyType": "ON_FAILURE",
    "restartPolicyMaxRetries": 3
  }
}
```

Set env vars in the Railway dashboard or via the Railway CLI:

```bash
railway variables set \
  JAMSESH_DB_DRIVER=postgres \
  JAMSESH_DB_DSN='${{Postgres.DATABASE_URL}}' \
  JAMSESH_STORAGE=/data/storage \
  JAMSESH_PORTAL_URL=https://jamsesh.up.railway.app \
  JAMSESH_TLS_MODE=behind_proxy \
  JAMSESH_OAUTH_GITHUB_CLIENT_ID=your-client-id \
  JAMSESH_OAUTH_GITHUB_CLIENT_SECRET=your-client-secret \
  JAMSESH_SHUTDOWN_GRACE_S=30
```

`${{Postgres.DATABASE_URL}}` is Railway's template syntax for referencing the
linked Postgres plugin's connection string. Adjust to your plugin's variable
name.

Railway does not natively support persistent volumes on the Hobby plan —
use Postgres for the database and consider GCS/S3 for bare-repo storage, or
upgrade to the Pro plan for volume support.

### Kubernetes with PVC

The portal is a single-pod deployment by design; clustered mode (multiple
replicas sharing a database and storage tier) is a future capability tracked in
`epic-cloud-native-deploy`. Run `replicas: 1`.

**`jamsesh-k8s.yaml`:**

```yaml
apiVersion: v1
kind: Namespace
metadata:
  name: jamsesh

---
apiVersion: v1
kind: PersistentVolumeClaim
metadata:
  name: jamsesh-data
  namespace: jamsesh
spec:
  accessModes:
    - ReadWriteOnce
  resources:
    requests:
      storage: 20Gi

---
apiVersion: v1
kind: ConfigMap
metadata:
  name: jamsesh-config
  namespace: jamsesh
data:
  JAMSESH_BIND: ":8443"
  JAMSESH_TLS_MODE: "behind_proxy"
  JAMSESH_DB_DRIVER: "postgres"
  JAMSESH_STORAGE: "/data/storage"
  JAMSESH_PORTAL_URL: "https://jamsesh.example.com"
  JAMSESH_SHUTDOWN_GRACE_S: "30"
  JAMSESH_DB_MAX_OPEN_CONNS: "25"
  JAMSESH_DB_MAX_IDLE_CONNS: "5"
  JAMSESH_DB_CONN_MAX_LIFETIME: "30m"

---
apiVersion: v1
kind: Secret
metadata:
  name: jamsesh-secrets
  namespace: jamsesh
type: Opaque
stringData:
  db-dsn: "host=postgres-svc user=jamsesh password=<pw> dbname=jamsesh sslmode=require"
  github-client-secret: "<your-secret>"

---
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jamsesh
  namespace: jamsesh
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jamsesh
  template:
    metadata:
      labels:
        app: jamsesh
    spec:
      terminationGracePeriodSeconds: 35
      containers:
        - name: portal
          image: ghcr.io/<owner>/jamsesh:v0.2.0
          ports:
            - containerPort: 8443
          envFrom:
            - configMapRef:
                name: jamsesh-config
          env:
            - name: JAMSESH_OAUTH_GITHUB_CLIENT_ID
              value: "your-client-id"
            - name: JAMSESH_DB_DSN_FILE
              value: /run/secrets/db-dsn
            - name: JAMSESH_OAUTH_GITHUB_CLIENT_SECRET_FILE
              value: /run/secrets/github-client-secret
          volumeMounts:
            - name: data
              mountPath: /data
            - name: secrets
              mountPath: /run/secrets
              readOnly: true
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8443
            initialDelaySeconds: 5
            periodSeconds: 10
            failureThreshold: 3
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8443
            initialDelaySeconds: 10
            periodSeconds: 30
            failureThreshold: 3
          resources:
            requests:
              memory: "128Mi"
              cpu: "100m"
            limits:
              memory: "512Mi"
      volumes:
        - name: data
          persistentVolumeClaim:
            claimName: jamsesh-data
        - name: secrets
          secret:
            secretName: jamsesh-secrets
            items:
              - key: db-dsn
                path: db-dsn
              - key: github-client-secret
                path: github-client-secret

---
apiVersion: v1
kind: Service
metadata:
  name: jamsesh
  namespace: jamsesh
spec:
  selector:
    app: jamsesh
  ports:
    - port: 8443
      targetPort: 8443
```

Key choices:

- `terminationGracePeriodSeconds: 35` — 30s portal grace + 5s buffer before
  SIGKILL. Matches `JAMSESH_SHUTDOWN_GRACE_S=30` in the ConfigMap.
- `readinessProbe` on `/readyz` — kubelet gates traffic until DB and storage
  are both healthy. `initialDelaySeconds: 5` gives the portal time to connect
  to Postgres before the first probe.
- `livenessProbe` on `/healthz` — restarts the pod if the process hangs.
- Secrets are mounted as files via `volumeMounts` and consumed via the `_FILE`
  env vars — Kubernetes Secret values never appear as plain env vars in this
  setup, which avoids `env`-based secret exposure in process listings.
- `replicas: 1` — the portal is single-instance by design; multiple replicas
  with a shared PVC (ReadWriteOnce) would cause mount conflicts.

Apply with:

```bash
kubectl apply -f jamsesh-k8s.yaml
kubectl -n jamsesh rollout status deployment/jamsesh
```

---

### Common setup issues

**Portal starts but `/healthz` returns connection refused**:
- Check `JAMSESH_BIND` matches the port you're hitting.
- In `behind_proxy` mode, the portal speaks plain HTTP — don't use `https://`
  when hitting it directly.

**OAuth login loop or callback errors**:
- Verify the callback URL registered with your OAuth provider exactly matches
  `https://<your-host>/auth/github/callback`.
- Check that `X-Forwarded-Proto: https` is set by your reverse proxy — the
  portal uses this to construct its own callback URL.

**Git push fails with `403 Forbidden`**:
- The HTTP Basic auth password for git push is the user's OAuth token (not
  their account password). Ensure the plugin is using the stored token.

**Database locked errors (SQLite)**:
- SQLite supports one writer at a time. If you're seeing lock contention,
  the portal may have multiple instances running against the same database.
  Confirm only one portal process is active.

**Bare repos missing after restart**:
- Ensure the `storage` directory (or the mounted volume in Docker) is
  persisted across restarts. Check that the volume mount in your `docker run`
  command covers both the database file and the storage directory.

---

## 14. Clustered mode (preview)

> **Preview status.** The router service, per-session Postgres leases, fencing
> tokens, and object-storage durability are shipped and tested. The one remaining
> gap before clustered mode is production-ready is **hydration handoff** — the
> ability to migrate a live session cleanly from one pod to another on demand
> (tracked in `epic-cloud-native-deploy`). Today, pod replacement causes a brief
> client-side `git fetch` retry loop until the new pod re-seeds the local repo
> from object storage. Use clustered mode for evaluation and staging workloads;
> use it in production only if you can tolerate a short client retry on pod
> restart.

### When to use clustered mode

The default jamsesh deployment is a single portal pod. A single pod is
adequate for teams of up to ~50 concurrent agents, scales well vertically, and
avoids all distributed-systems concerns. Clustered mode is appropriate when:

- You need horizontal scale-out beyond what a single host can provide.
- You need zero-downtime rolling upgrades without session disruption.
- You are running a multi-tenant managed service where isolation between
  tenants is enforced at the pod level.

### Prerequisites

- **Postgres** — SQLite is not supported in clustered mode. Advisory locks
  used by the per-session lease mechanism require Postgres 12+. See §5 for
  Postgres setup.
- **Object storage** — required for cross-pod bare-repo durability. Every push
  is mirrored to object storage before the git client receives a success
  response (RPO=0). Set `JAMSESH_OBJECT_STORAGE_URL` to one of the supported
  URL schemes (see §14 "Object storage (durability)" below). The portal refuses
  to start in clustered mode without this URL.
- **The `jamsesh-router` binary** — a separate Go binary in the same repo
  (`cmd/jamsesh-router`). Build it alongside the portal:
  ```bash
  go build -o jamsesh-router ./cmd/jamsesh-router
  ```

### Architecture overview

```
         Clients (agents + browsers)
                    │
         ┌──────────▼──────────┐
         │  jamsesh-router      │   consistent-hash reverse proxy
         │  (:8080, no TLS)    │   + round-robin for session-less routes
         └──────────┬──────────┘
                    │  session-ID-keyed sticky routing
         ┌──────────▼───────────────────────────┐
         │  portal pods (N replicas, :8443)      │
         │  shared Postgres + (future) object    │
         │  storage                              │
         └───────────────────────────────────────┘
```

The router extracts the session ID from every request:
- REST: from the URL path (`/api/orgs/{org}/sessions/{id}/...`)
- WebSocket: same path
- Git: same path (`/git/orgs/{org}/sessions/{id}.git/...`)
- MCP: from the `Jam-Session-Id` request header

It hashes the session ID onto the consistent-hash ring of healthy portal pods
and reverse-proxies the request. Requests without a session ID (e.g. `/healthz`,
`/auth/*`) are distributed round-robin across all pods. On a pod returning 503
the router invalidates its hint cache entry and retries once against the next
ring preference.

### Config knobs (`JAMSESH_ROUTER_*`)

| Env var | Default | Description |
|---|---|---|
| `JAMSESH_ROUTER_BIND` | `:8080` | Listen address for the router |
| `JAMSESH_ROUTER_DISCOVERY_MODE` | `static` | `static` or `kubernetes` |
| `JAMSESH_ROUTER_STATIC_PODS` | — | Comma-separated pod addresses, e.g. `10.0.0.1:8443,10.0.0.2:8443` (static mode) |
| `JAMSESH_ROUTER_KUBE_NAMESPACE` | — | Kubernetes namespace to watch (kubernetes mode) |
| `JAMSESH_ROUTER_KUBE_SERVICE_NAME` | — | Service name whose pod IPs to watch (kubernetes mode) |
| `JAMSESH_ROUTER_VNODES` | `150` | Consistent-hash virtual nodes per pod |
| `JAMSESH_ROUTER_HINT_CACHE_TTL` | `5m` | Soft-coordinator hint cache TTL |
| `JAMSESH_ROUTER_PROBE_INTERVAL` | `10s` | Readiness-probe interval (kubernetes mode) |
| `JAMSESH_ROUTER_SHUTDOWN_GRACE_S` | `30` | Graceful drain budget in seconds |

### Kubernetes deployment

The following YAML deploys a two-replica portal cluster plus the router as a
separate Deployment. Adapt namespaces, image references, resource limits, and
secrets management to your environment.

```yaml
# jamsesh-cluster.yaml
---
# Namespace
apiVersion: v1
kind: Namespace
metadata:
  name: jamsesh

---
# Portal ConfigMap
apiVersion: v1
kind: ConfigMap
metadata:
  name: jamsesh-portal-config
  namespace: jamsesh
data:
  JAMSESH_TLS_MODE: "behind_proxy"
  JAMSESH_DB_DRIVER: "postgres"
  JAMSESH_SHUTDOWN_GRACE_S: "30"

---
# Portal Deployment (multi-replica)
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jamsesh-portal
  namespace: jamsesh
spec:
  replicas: 2
  selector:
    matchLabels:
      app: jamsesh-portal
  template:
    metadata:
      labels:
        app: jamsesh-portal
    spec:
      terminationGracePeriodSeconds: 35
      containers:
        - name: portal
          image: ghcr.io/<owner>/jamsesh:v0.3.0
          ports:
            - name: http
              containerPort: 8443
          envFrom:
            - configMapRef:
                name: jamsesh-portal-config
          env:
            - name: JAMSESH_DB_DSN
              valueFrom:
                secretKeyRef:
                  name: jamsesh-secrets
                  key: db_dsn
            - name: JAMSESH_GITHUB_CLIENT_ID
              valueFrom:
                secretKeyRef:
                  name: jamsesh-secrets
                  key: github_client_id
            - name: JAMSESH_GITHUB_CLIENT_SECRET
              valueFrom:
                secretKeyRef:
                  name: jamsesh-secrets
                  key: github_client_secret
            - name: JAMSESH_SESSION_SECRET
              valueFrom:
                secretKeyRef:
                  name: jamsesh-secrets
                  key: session_secret
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8443
            initialDelaySeconds: 5
            periodSeconds: 10
          livenessProbe:
            httpGet:
              path: /healthz
              port: 8443
            initialDelaySeconds: 15
            periodSeconds: 30

---
# Portal Service (ClusterIP — only the router talks to pods directly)
apiVersion: v1
kind: Service
metadata:
  name: jamsesh-portal
  namespace: jamsesh
spec:
  selector:
    app: jamsesh-portal
  ports:
    - port: 8443
      targetPort: 8443
  # ClusterIP is intentional: external traffic enters via the router, not here.

---
# Router Deployment
apiVersion: apps/v1
kind: Deployment
metadata:
  name: jamsesh-router
  namespace: jamsesh
spec:
  replicas: 1
  selector:
    matchLabels:
      app: jamsesh-router
  template:
    metadata:
      labels:
        app: jamsesh-router
    spec:
      serviceAccountName: jamsesh-router  # needs pod list/watch on the namespace
      terminationGracePeriodSeconds: 35
      containers:
        - name: router
          image: ghcr.io/<owner>/jamsesh-router:v0.3.0
          ports:
            - name: http
              containerPort: 8080
          env:
            - name: JAMSESH_ROUTER_BIND
              value: ":8080"
            - name: JAMSESH_ROUTER_DISCOVERY_MODE
              value: "kubernetes"
            - name: JAMSESH_ROUTER_KUBE_NAMESPACE
              value: "jamsesh"
            - name: JAMSESH_ROUTER_KUBE_SERVICE_NAME
              value: "jamsesh-portal"
            - name: JAMSESH_ROUTER_SHUTDOWN_GRACE_S
              value: "30"
          readinessProbe:
            httpGet:
              path: /readyz
              port: 8080
            initialDelaySeconds: 3
            periodSeconds: 5

---
# Router Service (LoadBalancer — external entry point)
apiVersion: v1
kind: Service
metadata:
  name: jamsesh-router
  namespace: jamsesh
spec:
  type: LoadBalancer
  selector:
    app: jamsesh-router
  ports:
    - port: 443
      targetPort: 8080
      # TLS termination handled upstream (e.g. cloud LB or an ingress with
      # cert-manager). The router speaks plain HTTP internally.

---
# RBAC: allow the router to list/watch pods
apiVersion: v1
kind: ServiceAccount
metadata:
  name: jamsesh-router
  namespace: jamsesh
---
apiVersion: rbac.authorization.k8s.io/v1
kind: Role
metadata:
  name: jamsesh-router
  namespace: jamsesh
rules:
  - apiGroups: [""]
    resources: ["pods"]
    verbs: ["get", "list", "watch"]
---
apiVersion: rbac.authorization.k8s.io/v1
kind: RoleBinding
metadata:
  name: jamsesh-router
  namespace: jamsesh
subjects:
  - kind: ServiceAccount
    name: jamsesh-router
roleBinding:
  apiGroup: rbac.authorization.k8s.io
  kind: Role
  name: jamsesh-router
```

Apply with:

```bash
kubectl apply -f jamsesh-cluster.yaml
kubectl -n jamsesh rollout status deployment/jamsesh-portal
kubectl -n jamsesh rollout status deployment/jamsesh-router
```

### Observability

The router exposes Prometheus metrics at `/metrics` on its bind address.
Scraped metric families:

| Metric | Type | Description |
|---|---|---|
| `jamsesh_router_decisions_total{result}` | counter | Routing decisions; result ∈ {hit_cache, hit_ring, fallback, empty_ring, retry, error_503} |
| `jamsesh_router_ring_size` | gauge | Current pod count in the consistent-hash ring |
| `jamsesh_router_ring_rebalances_total` | counter | Ring rebalances (pod set changes) |
| `jamsesh_router_probe_failures_total{addr}` | counter | Readiness-probe failures per pod address |

Standard Go runtime and process metrics (`go_goroutines`, `go_memstats_*`,
`process_cpu_seconds_total`) are also included.

### What is not yet in place (preview limitations)

The following capability is planned but not yet landed:

- **Hydration handoff** — when the ring rebalances and a session moves to a
  new pod, the new pod has no local repo cache yet. It can serve read-only
  requests (digest, refs, comments) immediately from Postgres, but the first
  push to the new pod re-seeds the local repo from object storage, which adds
  a brief one-time latency. Future work will pre-hydrate the cache on lease
  acquisition so the new pod is push-ready before the router redirects traffic.

Until hydration handoff lands, clustered mode handles pod replacement correctly
but with a possible one-push latency on first contact with the new pod. Clients
retry automatically on 503 (the router retries once; git clients retry on the
next push). For rolling upgrades, drain the old pod (SIGTERM causes a 30s
graceful shutdown) before the new pod goes live to minimize the transition
window.

### Object storage (durability)

Object storage is the system of record for bare repos in clustered mode. Every
push is mirrored to object storage before the git client receives a success
response — this is the RPO=0 contract. Local disk on each pod is a working
cache; its contents are bounded by lease tenure.

**Required env vars:**

| Env var | Purpose |
|---|---|
| `JAMSESH_OBJECT_STORAGE_URL` | Object-storage URL (required in clustered mode) |
| `JAMSESH_OBJECT_STORAGE_REGION` | Provider region (required for AWS S3; optional for others) |
| `JAMSESH_OBJECT_STORAGE_ENDPOINT_URL` | Endpoint override for S3-compatible services (R2, B2, MinIO) |
| `JAMSESH_OBJECT_STORAGE_PATH_STYLE` | Force path-style addressing — `true` for MinIO/Ceph, `false` otherwise |
| `JAMSESH_OBJECT_STORAGE_SYNC_QUEUE_SIZE` | Max concurrent in-flight uploads per session (default `256`) |

**Supported URL schemes:**

| Scheme | Provider | Example |
|---|---|---|
| `s3://bucket/prefix` | AWS S3 | `s3://my-jamsesh-bucket/sessions` |
| `s3-compatible://bucket/prefix` | Cloudflare R2, Backblaze B2, MinIO, Ceph | `s3-compatible://my-bucket` |
| `gs://bucket/prefix` | Google Cloud Storage | `gs://my-jamsesh-bucket/sessions` |
| `azblob://account/container/prefix` | Azure Blob Storage | `azblob://myaccount/jamsesh/sessions` |

**AWS S3 (recommended for AWS deployments):**

Use IRSA (IAM Roles for Service Accounts) on EKS — no static credentials needed.

```bash
JAMSESH_OBJECT_STORAGE_URL=s3://my-jamsesh-bucket/sessions
JAMSESH_OBJECT_STORAGE_REGION=us-east-1
# Credentials: IRSA annotation on the ServiceAccount, or IAM instance role
```

Required S3 permissions: `s3:PutObject`, `s3:GetObject`, `s3:DeleteObject`,
`s3:ListBucket` scoped to the configured bucket.

**Cloudflare R2:**

```bash
JAMSESH_OBJECT_STORAGE_URL=s3-compatible://my-r2-bucket
JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=https://<account-id>.r2.cloudflarestorage.com
AWS_ACCESS_KEY_ID=<r2-access-key-id>
AWS_SECRET_ACCESS_KEY=<r2-secret-access-key>
```

R2 does not require a region setting. Credentials are R2-specific API tokens
created in the Cloudflare dashboard under R2 → Manage API Tokens.

**MinIO (self-hosted):**

```bash
JAMSESH_OBJECT_STORAGE_URL=s3-compatible://my-bucket/sessions
JAMSESH_OBJECT_STORAGE_ENDPOINT_URL=http://minio.internal:9000
JAMSESH_OBJECT_STORAGE_PATH_STYLE=true
AWS_ACCESS_KEY_ID=<minio-access-key>
AWS_SECRET_ACCESS_KEY=<minio-secret-key>
```

Set `JAMSESH_OBJECT_STORAGE_PATH_STYLE=true` for MinIO and self-hosted Ceph.
For local development MinIO (plain HTTP), pass an `http://` endpoint URL.

**Google Cloud Storage (GKE Workload Identity — recommended):**

```bash
JAMSESH_OBJECT_STORAGE_URL=gs://my-jamsesh-bucket/sessions
# Credentials: GKE Workload Identity annotation on the KSA, or ADC via
# GOOGLE_APPLICATION_CREDENTIALS=/path/to/service-account-key.json
```

Required GCS permissions: `storage.objects.create`, `storage.objects.get`,
`storage.objects.delete`, `storage.buckets.list` on the configured bucket.
Grant via IAM with the `Storage Object User` role (or a custom role scoped
to the bucket).

**Azure Blob Storage (AKS Workload Identity — recommended):**

```bash
JAMSESH_OBJECT_STORAGE_URL=azblob://myaccount/jamsesh/sessions
# Credentials: AKS Workload Identity federated identity + Service Principal,
# or static credentials via:
#   AZURE_CLIENT_ID, AZURE_CLIENT_SECRET, AZURE_TENANT_ID
```

Required Azure permissions: `Storage Blob Data Contributor` role scoped to the
storage account container.

**Cost model:**

Object storage costs for jamsesh are dominated by API call counts rather than
storage volume. A session produces roughly 5–20 objects per push (loose git
objects + a manifest update). At heavy use (~100 pushes/session/day across 10
active sessions):

- ~$0.05 per active session per day at AWS S3 / GCS pricing.
- Storage cost is negligible — a busy session accumulates a few MB of git objects.
- Cloudflare R2 eliminates egress costs, making it cost-efficient when
  portal pods and clients are outside the same AWS/GCP region.

Monitor `jamsesh_object_storage_uploads_total{result="error"}` to detect
object-storage errors that would degrade push reliability.
