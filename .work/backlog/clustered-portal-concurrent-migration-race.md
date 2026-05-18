---
id: clustered-portal-concurrent-migration-race
created: 2026-05-18
tags: [testing, infra, portal, postgres]
---

When portalcluster.Start spins up two portal pods in parallel against a fresh Postgres database, both pods race to run SQL migrations concurrently; Postgres reports "duplicate key value violates unique constraint pg_type_typname_nsp_index" and pod startup exits with code 1, blocking all clustered-mode e2e tests (TestStaleFencingTokenRejected, TestLeaseAlreadyHeld, and others). The root cause is the errgroup in portalcluster.Start firing both portal.Start calls simultaneously — each calls db.Open which runs migrate.Up, and concurrent DDL (e.g. CREATE TYPE) conflicts in Postgres. Fix options: serialize migration across pods (one pod migrates first, others skip or wait on a migration lock), or have portalcluster.Start start one pod, wait for its healthz, then start the rest.
