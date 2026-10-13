# Deploy with automatic PostgreSQL migrations

The backend image contains two independent entrypoints:

- `/app/urlo`: Web/gRPC service; never applies schema migrations.
- `/app/urlo-migrate`: deployment command with embedded Goose SQL migrations.

The release gate runs the migration command in a one-shot Kubernetes Job using
**the exact immutable image digest** that will then be rolled out. No SQL CLI,
Go compiler, config file, S3 credentials or migration volume is needed in the Job.

## Prerequisites

- An existing backend Deployment and its container name, namespace and explicit
  kubeconfig context. The script updates only that container image; it does not
  provision a cluster, database, Deployment, Secrets or relation configuration.
- A published image built from this version of the Dockerfile, including both
  binaries. PR image builds do not publish; use a published release digest or
  publish a staging image through your existing image pipeline.
- A namespace Secret holding a **migration-role** PostgreSQL DSN under key `dsn`
  (override with `--secret-key` locally). The role needs DDL/ownership for these
  tables, not just the application's DML privileges. Web pods keep a separate
  application-role Secret. Never put the migration DSN in a CLI argument or Git.
- A direct PostgreSQL connection or a session-pooling endpoint. **Transaction
  pooling is incompatible with PostgreSQL session advisory locks.** Configure TLS
  in the DSN; optionally supply a Secret with `ca.crt` via `--pg-ca-secret`.
- Registry access for the Job. Explicit `imagePullSecrets`, node selectors and
  tolerations are copied from the Deployment template. If registry credentials
  exist only on its ServiceAccount, configure explicit imagePullSecrets on the
  template (the Job deliberately does not borrow that account's API privileges).
- NetworkPolicy permits pods labeled `app.kubernetes.io/name: urlo-migrate` to
  resolve DNS and connect to PostgreSQL. Job pods do not inherit service-facing
  labels, avoiding accidental HTTP traffic to the migration process.
- The release identity can get/patch/watch this Deployment and create/get Jobs
  in the namespace. It does not need to read Secret values. Use a dedicated,
  least-privilege kubeconfig rather than a cluster-admin credential.

## Release locally or from your deployment pipeline

Requires Python 3, kubectl, and a preconfigured kubeconfig. Use a kubectl version
within one minor of the server. Execute on a trusted Unix runner:

```bash
python3 scripts/deploy-kubernetes.py \
  --context your-context --namespace your-namespace \
  --deployment urlo --container urlo \
  --image 'ghcr.io/kongken/urlo@sha256:<published-64-character-digest>' \
  --migration-secret urlo-postgres-migration
```

The placeholder digest is intentionally invalid until replaced with the real
published digest. No live cluster/namespace defaults are inferred.

The script:

1. Reads the existing Deployment and verifies the target container.
2. Creates a uniquely named Job with `backoffLimit: 0` and a bounded deadline.
   It injects the DSN through `secretKeyRef`, runs as a non-root UID, disables
   service-account token mounting, and uses a read-only root filesystem.
3. Waits for Job completion. Failure, timeout or Kubernetes API errors stop the
   release **without patching the Deployment**. Failed Job names are reported
   for diagnostics; Pod logs/Secret values are not automatically dumped.
4. Tests the Deployment UID and original pod template with an optimistic JSON
   patch before replacing the target image. A concurrent config/release change
   fails the patch instead of silently overwriting it. Status-only updates do
   not invalidate the template guard.
5. Waits for Deployment rollout completion. A post-patch rollout failure is
   reported, **not** automatically undone, and never runs a Down migration.

Defaults: migration command deadline 300s including lock wait; Job deadline 330s;
release gate waits at most 360s for the Job. `--migration-timeout` and
`--rollout-timeout` accept 1..3600 seconds. Finished Jobs expire after one hour;
inspect diagnostics before TTL cleanup. Retrying a release creates a fresh Job,
while Goose history prevents reapplying already-completed migrations.

## GitHub Actions entrypoint

[Deploy (migration gate)](../.github/workflows/deploy.yml) is an explicit
`workflow_dispatch` release workflow. Image publication, pushes and PR CI **do not
trigger production deployment**. Select the published image digest and target
context/namespace/Deployment/container, migration Secret and a compatible
`kubectl_version` when dispatching it. The optional `pg_ca_secret` supplies TLS CA.

Configure a **protected GitHub Environment** with a `KUBE_CONFIG` secret containing
raw kubeconfig YAML, required approvers and deployment-branch restrictions.
Keep this credential at Environment scope, **not repository scope**; otherwise
an arbitrary Environment name could bypass the intended approval boundary.
The workflow writes a mode-0600 temporary kubeconfig and removes it on completion.
The PostgreSQL DSN stays in the cluster Secret and is not passed to GitHub Actions.

Workflow concurrency serializes releases for the same context/namespace/Deployment
without canceling an in-progress migration. Other deployment systems must also use
one release writer per Deployment. The database lock serializes migration DDL,
not independent deployment controllers or arbitrary administrative SQL.

## Migration command and failure semantics

```bash
make build-migrate
# DSN supplied through a secret-injected environment:
bin/urlo-migrate --timeout=5m
# Equivalent developer entrypoint:
make postgres-migrate
```

The tool uses Goose v3.29.0, the existing `goose_db_version` history table and
PostgreSQL session advisory lock ID `4097083626`. Actual migration application
holds the lock on the same SQL connection as its DDL, bounded by the command
context; release cleanup also has a deadline. Duplicate concurrent Jobs serialize
and the later one observes applied versions. Use this command for automated Up
operations; do not concurrently run an unlocked external Goose Up/Down command.

Only Up is exposed. Repeated runs are no-ops; databases newer than the bundled
migration set are refused, and malformed/missing DSNs or migration failures exit
nonzero. Error messages redact connection strings/passwords. SIGINT/SIGTERM cancel
queries and lock waits. Each SQL migration is transactional unless explicitly
annotated otherwise by Goose; previously completed files remain applied if a
later migration fails. Recovery is an explicit fix-forward release, not automatic
schema rollback.

Schema updates must remain compatible with the still-running old application
(expand/contract for breaking changes). A successful schema migration does not
prove backfill is complete or the new application is healthy. Keep the S3-read
canary and rollback procedure in [storage operations](storage.md).

The tool lives in a separate `cmd/urlo-migrate/go.mod` so Goose deployment-tool
dependencies do not upgrade the Web service's gRPC/OTel graph. Its root-module
replacement is used only to bundle the canonical SQL files, not duplicate them.
Root `go test ./...` intentionally skips it; run `make test-migrate` explicitly.
`make test-deploy` verifies the release gate with mocked kubectl responses. CI
runs both plus real isolated PostgreSQL tests. No production/staging release has
been executed merely by adding this workflow.
