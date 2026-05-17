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
| `JAMSESH_OAUTH_GITHUB_BASE_URL` | `oauth.github.base_url` | _(none)_ | Override GitHub OAuth base URL for testing; leave unset in production |

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

### Metrics and alerting

A Prometheus metrics endpoint (`/metrics`) is planned for a future release.
In the meantime, the JSON log stream is the primary observability surface.
Key signals to alert on via log aggregation:

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

### When migrations are required

Every release notes document lists any schema migrations included. All
migrations are backward-safe (additive: new columns, new tables). There are
no destructive migrations in v1 — the portal can always start against a
database from the previous release.

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
