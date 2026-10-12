# S3 primary records + PostgreSQL owner index

PostgreSQL is **not** a replacement `storage.driver`. Production links still
use `storage.driver: s3`; memory remains available for development/tests.

S3 JSON remains authoritative for code uniqueness, destination, owner, expiry,
disabled status and the existing best-effort visit count. PostgreSQL stores
`(link_space, code, owner_id, link_created_at, indexed_at)` for owner queries,
plus an internal Ent row `id`.
No users, tags, folders, ACLs or complete link replicas are added in this phase.
Redis click recording and rate limiting are unchanged.

## Ent DAO and schema changes

The PostgreSQL DAO uses **Ent v0.14.6**, not handwritten row SQL or GORM.
The underlying driver remains pgx through its `database/sql` bridge over the
bounded `pgxpool`; Ent owns the bridge, and startup/shutdown manages both clients.

- Model: [`OwnerLinkIndex`](../internal/relations/ent/schema/owner_link_index.go).
- Generated builders: `internal/relations/ent/`; generator pinned as a Go tool
  in `go.mod`. Edit the schema, run `make ent`, and commit the generated changes.
  CI verifies that regeneration produces no diff.
- DAO: `internal/relations/postgres/index.go`. Reads, conditional upserts,
  deletes and transactions use Ent builders. Only PostgreSQL's per-key advisory
  lock uses Ent's raw execution escape hatch, within the same transaction.
- Schema deployment remains **versioned Goose SQL**. The service never calls
  Ent `Schema.Create`; probing and data backfill do not create or alter tables.

Migration `00002_owner_link_index_ent_id.sql` upgrades existing V1 tables:
it adds a bigint identity row ID, makes it the primary key, and retains
`(link_space, code)` as a unique constraint. Existing refs, case-sensitive
collations, owner paging indexes and the previous release's conflict target
are preserved. Run this migration **before deploying the Ent DAO**.
The Down migration preserves owner refs; after downgrading the schema, use the
pre-Ent binary. Normally roll back the binary/read source without reverting DDL.
The ALTER operations lock/reindex the table; schedule large-table upgrades in
an appropriate maintenance window and validate duration on representative data.

This database row ID is disposable and changes if an index row is deleted and
recreated. It is **not** an immutable S3 `link_id`, and must not be used to bind
tags, folders, permissions or other persistent business relations.

## Enable safely

1. Provision a PostgreSQL database and separate migration/application roles.
   The migration role needs DDL permissions; the application role needs
   SELECT/INSERT/UPDATE/DELETE on `owner_link_index`.
2. Inject a PostgreSQL DSN as `URLO_POSTGRES_DSN` using deployment secrets.
   Configure TLS as required by your platform. Never commit or print credentials.
3. Install the pinned migration CLI (independent of the application's Go module):

   ```bash
   go install -tags='no_mysql no_sqlite3 no_ydb no_mssql no_libsql no_azuresql no_clickhouse no_vertica' github.com/pressly/goose/v3/cmd/goose@v3.29.0
   ```

4. Run the migration once in a deployment job, **not in every Web pod**:

   ```bash
   make postgres-migrate
   make postgres-status
   ```

   These targets read `URLO_POSTGRES_DSN`; `GOOSE=/path/to/goose` can override
   the executable. The application only probes the table; it does not migrate it.
5. Configure S3 as usual, then enable dual-write/backfill with S3 reads:

   ```yaml
   storage:
     driver: s3
     s3:
       config_name: links
       prefix: urlo/links

   relations:
     driver: postgres
     link_space: urlo-production
     owner_index_read_source: s3
     buffer_size: 1024
     reconcile_interval_seconds: 300
     postgres:
       dsn_env: URLO_POSTGRES_DSN
       max_conns: 10
       connect_timeout_seconds: 3
       query_timeout_ms: 1000
   ```

   `relations.driver` defaults to `none`. When enabled, `storage.driver=s3`
   and a nonempty, stable `link_space` are required. A space identifies one
   bucket/prefix code namespace; replicas of that namespace must agree.
   Core's `store.db` remains MySQL-only and must not be used for PostgreSQL.
6. Observe completed reconciliation logs and compare S3/SQL owner results.
   `owner_index_read_source: s3` maintains the new index but still queries old
   pointers; it does not automatically perform shadow comparisons.
7. Once backfill is complete, change `owner_index_read_source` to `postgres`
   and restart the service. The frontend follows all owner-list pages.

S3 owner pointers continue to be written in this release. Rollback can switch
reads to `s3`, or disable relations entirely, without deleting PostgreSQL rows.
Do not remove pointers while relying on this rollback path. Cursor tokens are
read-source-specific: clients must restart pagination after a switch.

## Consistency and repair

- Primary `Create`, `Update` and `Delete` first use S3, then enqueue a key for
  refresh without blocking. S3 success is returned even when SQL is down or
  the bounded queue is full. Resolution, availability, visit counting and
  ownership checks do not call PostgreSQL.
- Expiry cleanup also uses the non-blocking notification path, including gRPC
  deletion. Only owner-list queries depend on SQL when PostgreSQL reads are on.
- The queue is in-process and not durable. Failed/dropped refreshes are repaired
  by the next full reconciliation; there is no guaranteed read-your-writes or
  fixed visibility-latency SLO. New links may temporarily be absent from lists.
- An immediate reconciliation starts asynchronously on startup; subsequent
  passes run at `reconcile_interval_seconds` (default 300). They enumerate S3
  **primary JSON keys**, not owner pointers, then revalidate all indexed codes
  for deleted/reused objects. Scans are paginated and are not whole-bucket snapshots.
- Refreshes take a PostgreSQL transaction-scoped per-key advisory lock before
  reading the current S3 object. Duplicate/late notifications and multiple
  replicas do not apply old event bodies. A concurrent S3 mutation can still
  temporarily lag, so periodic repair remains necessary.
- S3 NotFound or an anonymous owner removes only the disposable owner ref.
  Other S3 errors leave it unchanged. Lists rehydrate current S3 records,
  check owner again, skip expired/deleted refs, and retain disabled links.
- Hydration uses at most eight concurrent reads, a candidate batch of at most
  100, and a scan budget of `max(500, page_size * 10)`. Short/empty pages can
  have a continuation token; clients must keep paging until the token is empty.
- Queue, refresh and reconciliation counters are available through `Store.Stats`
  and reported in reconciliation logs, including failures. They are not yet
  exported as a dedicated Prometheus metric set.

SQL/list operations have `query_timeout_ms` deadlines. Refresh includes the
S3 read within that deadline; adjust it to match observed S3 latency. Connection
count is explicitly capped per process; include replicas and jobs in the database
connection budget. A source/SQL outage stops a reconciliation pass and the next
scheduled pass retries; a failed queue item is not an immediate durable retry.

A syntactically invalid/missing DSN or invalid configuration fails startup.
A temporarily unavailable PostgreSQL server or missing migration logs a warning
but allows the S3 service to start. PostgreSQL owner queries return `503` when
the database or their required table/columns are unavailable; S3-read mode
continues using pointers. A V1 table may still serve old owner refs, but Ent
refreshes require migration 00002 and cannot repair new data until it is applied. `/health` and redirects do not become dependent
on the relation database. SIGINT/SIGTERM cancel workers and close their pool;
Core does not expose HTTP/gRPC draining, so full server graceful shutdown remains
an existing framework limitation. Queue work is intentionally not drained on exit.

## Tests

```bash
go test -race ./...
# Use a dedicated test database/user able to create schemas:
URLO_TEST_POSTGRES_DSN='postgres://test@localhost:5432/urlo_test?sslmode=disable' \
  go test -race ./internal/relations/postgres
```

PostgreSQL integration tests create and drop isolated, randomly named schemas,
apply the actual migrations' Up SQL, and test the public adapter/module interfaces
through a real Ent client. Additional tests cover upgrading existing V1 rows,
rollback/reapply, no automatic schema creation, unchanged-ref upserts at database
time precision, and transaction timeout/lock release.
They skip when `URLO_TEST_POSTGRES_DSN` is unset. Do not point them at production.
CI provisions PostgreSQL 17, validates/applies migrations using goose, and runs
these tests with the race detector. Local verification used PostgreSQL 16.

## Deferred work

- Persistent tag/folder/share relations require an immutable link-generation
  ID in S3 before association: codes can be deleted and reused. Do not attach
  business-data foreign keys with cascading deletes to this disposable index.
- No S3 event queue is configured yet. Current `IncrVisit` rewrites JSON per
  visit, so blindly subscribing every PUT would also process click traffic.
- The existing S3 counter's non-CAS read-modify-write races and gRPC ownership
  limitations are not fixed by the owner index. See [gRPC](api/grpc.md).
- Large-bucket scan cost, target-provider compatibility, visibility-latency
  targets, production sizing and backup procedures still need deployment-specific
  validation. See [research](research/postgresql-relations.md).
