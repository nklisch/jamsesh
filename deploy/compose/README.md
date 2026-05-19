# jamsesh — Docker Compose self-host template

Turn-key quickstart for running jamsesh on your own server. For the full
operator reference — TLS modes, OAuth callbacks, database options, monitoring,
and backup — see [docs/SELF_HOST.md](../../docs/SELF_HOST.md). For project
overview and development setup, see the [root README](../../README.md).

## Prerequisites

- Docker 24+ and Docker Compose v2 (`docker compose version`)
- A domain name with an A/AAAA record pointing at this host (ports 80 and 443
  must be reachable from the internet for Let's Encrypt to issue a certificate)

For a quick local trial without a domain or TLS, use the `docker run` snippet
in the root README instead.

## Quickstart

```bash
git clone https://github.com/nklisch/jamsesh
cd jamsesh/deploy/compose
cp .env.example .env
$EDITOR .env   # set JAMSESH_DOMAIN; configure OAuth or email auth
docker compose up -d
```

After a few seconds the portal is reachable at `https://<JAMSESH_DOMAIN>`.
Caddy provisions a Let's Encrypt certificate automatically on first request.

## Choosing SQLite vs Postgres

**SQLite (default)** — zero extra infrastructure; works well for single-node
deploys with moderate session volume. Data lives in the `jamsesh_data` named
volume at `/data/jamsesh.db`.

**Postgres (optional)** — activate the `postgres` profile when you want a
separate database server or are planning a HA setup. To switch:

1. In `.env`, uncomment `POSTGRES_USER`, `POSTGRES_PASSWORD`, `POSTGRES_DB`,
   `JAMSESH_DB_DRIVER`, and `JAMSESH_DB_DSN`.
2. Bring the stack up with the profile flag:

   ```bash
   docker compose --profile postgres up -d
   ```

The portal retries DB connections at startup, so a brief Postgres init delay
is handled automatically.

## Image version pinning

`.env.example` ships with `JAMSESH_VERSION=v0.1.0`. Pinning a specific tag
gives you reproducible, intentional upgrades — you decide when to move forward.
To upgrade:

```bash
# Bump the version in .env, then:
docker compose pull
docker compose up -d
```

Migrations run automatically when the portal starts.

## Troubleshooting

**Let's Encrypt cert provisioning fails.**
Check that `JAMSESH_DOMAIN` resolves to this host's public IP and that ports
80 and 443 are open in any firewall or cloud security group. Caddy must be able
to complete an ACME HTTP-01 challenge on port 80.

**"No auth configured" on first login.**
The portal starts cleanly without OAuth or email, but no login method is
available. Configure at least one: GitHub OAuth (simpler for teams) or
magic-link email (simpler for single operators). See
[docs/SELF_HOST.md §4](../../docs/SELF_HOST.md#4-oauth-callback-urls) for
OAuth setup and §6 for email providers.

**Volume permission errors.**
Docker named volumes inherit ownership from the first container write — they
work out of the box. If you mount a host directory instead of a named volume,
ensure the portal user (`nobody`, UID 65534) has write access:

```bash
sudo chown -R 65534:65534 /path/to/your/host/dir
```

**Postgres service not starting.**
Ensure you passed `--profile postgres` to `docker compose up -d`. Without the
flag, the `postgres` service is excluded from the stack.

## Upgrading

```bash
# Edit JAMSESH_VERSION in .env, then:
docker compose pull
docker compose up -d
```

The portal runs database migrations automatically on startup. No manual
migration step is needed.
