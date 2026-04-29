# CLAUDE.md

Guidance for Claude Code working in this repo.

## Project

`urlo` — a URL shortener service written in Go. Module path: `github.com/kongken/urlo`.

Built on the `butterfly.orx.me/core` app framework (provides config loading, HTTP/gRPC server wiring, logging, OTel). Exposes:

- **REST/JSON** via Gin (`internal/http`)
- **gRPC + Connect** via generated stubs (`pkg/proto/urlo/v1`)

## Layout

```
cmd/urlo/             # main entrypoint; wires app.Config + builds storage
internal/config/      # ServiceConfig: BaseURL, StorageConfig, etc.
internal/http/        # Gin REST routes (delegates to url.Service)
internal/url/         # Core URL service: Service, Store interface
internal/url/s3store/ # S3-backed Store implementation
proto/urlo/v1/        # .proto sources (buf-managed)
pkg/proto/urlo/v1/    # generated protobuf/grpc/connect Go code
```

## Commands

| Task | Command |
|------|---------|
| Run locally | `make run` (uses `config.yaml`) |
| Build binary | `make build` → `bin/urlo` |
| Tests | `go test ./...` |
| Tests w/ coverage | `go test -race -covermode=atomic -coverprofile=coverage.out ./...` |
| Regenerate proto | `make proto` (requires `buf`) |
| Lint proto | `make proto-lint` |
| Tidy modules | `make tidy` |
| Docker build | `docker build -t urlo .` |

## Conventions

- **Storage drivers** are pluggable via `url.Store` interface. Adding a new backend: implement `Store` (see `internal/url/memory.go`, `internal/url/s3store/`), then wire it into `buildStore` in `cmd/urlo/main.go` with a new `driver` value.
- **S3 client** comes from `butterfly.orx.me/core/store/s3` — configured under `store.s3.<name>` in `config.yaml`, referenced by `storage.s3.config_name`.
- **Proto changes**: edit files under `proto/urlo/v1/`, then `make proto`. Do not hand-edit files under `pkg/proto/`.
- **Tests**: prefer table-driven tests. `s3store` tests use a mockable S3 interface — keep the interface minimal so mocks stay easy.
- **Go version**: 1.26 (see `go.mod`). The Dockerfile and CI both pin to 1.26.

## CI

GitHub Actions (`.github/workflows/go.yml`) runs build + tests with race detector and uploads coverage to Codecov on push/PR to `main`. Docker images are published via `docker-publish.yml`.
