# urlo

[![Go](https://github.com/kongken/urlo/actions/workflows/go.yml/badge.svg)](https://github.com/kongken/urlo/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/kongken/urlo/branch/main/graph/badge.svg)](https://codecov.io/gh/kongken/urlo)

URL shortener with an HTTP/JSON API, a gRPC API, optional Google sign-in, and a React (Vite + TypeScript) web UI.

**功能概览（与代码一致）**：短链生成（随机或自定义码、可选 TTL、可配置随机码长度）、302 跳转、访问计数、按所有者管理链接、链接更新与启用/停用、点击事件（可选 Redis Stream）、聚合分析、按 IP 的创建频率限制、内存或 S3 持久化。详细 HTTP 约定见 [`api.md`](api.md)。

## Features

- **Core**: Create short links (`POST /api/v1/urls`), resolve JSON (`GET /api/v1/urls/:code`, `…/lookup`), stats without bumping visits (`GET …/stats`), public redirect (`GET /:code`), delete (`DELETE …/:code`).
- **Customization**: `custom_code`, `ttl_seconds`, `code_length` (6–32 for auto-generated codes); `GET /api/v1/urls/availability?code=` checks whether a custom code is free.
- **Auth (optional)**: Google ID token exchange → HTTP-only session cookie (`POST /api/v1/auth/google`, `logout`, `me`). When Google client ID is not configured, auth routes return **503** and all links are anonymous.
- **Ownership**: Logged-in users own new links; `GET /api/v1/urls` lists non-expired owned links. Anonymous links can be read/updated/deleted by anyone with the code; owned links require the matching session.
- **Moderation**: `PATCH …/status` and `GET …/status` disable or re-enable a link; disabled codes return **404** on resolve/redirect but can still be inspected via stats/clicks/analytics when permitted.
- **Clicks & analytics**: Optional Redis Streams click log; `GET …/clicks` paginates raw events; `GET …/analytics` aggregates by `day`, `country`, or `referer` (country uses `(unknown)` when empty in the event).
- **Infra**: [Butterfly](https://butterfly.orx.me/core) app bootstrap (`cmd/urlo`), YAML config (`config.yaml`) for `base_url`, `code_length`, `storage` (memory or S3), `rate_limit`, `auth`, `clicks`.
- **Web UI** (`front/`): landing shorten flow, dashboard (my links), per-link analytics, settings (API base URL override in `localStorage`).

## Repository layout

| Path | Role |
|------|------|
| `cmd/urlo` | Service entrypoint (HTTP + gRPC registration) |
| `internal/http` | Gin routes, JSON DTOs, auth wiring |
| `internal/url` | Domain logic and store abstraction |
| `internal/clicks` | Click capture + UA enrichment |
| `proto/urlo/v1` | Protobuf API (gRPC subset) |
| `pkg/proto/urlo/v1` | Generated Go protobuf / Connect code |
| `front/` | Vite + React SPA |
| `api.md` | HTTP/JSON reference |

## Quick start (local)

```bash
make run
```

Uses `BUTTERFLY_CONFIG_FILE_PATH=./config.yaml` (see `Makefile`). Defaults: in-memory store, no auth, no click logging.

**gRPC / second port**: Exposed by the Butterfly runtime alongside HTTP (commonly HTTP `8080` and gRPC `9090` in examples—confirm in your deployment).

**Web UI**: From `front/`, install deps and run the dev server; set `VITE_API_BASE_URL` to the API origin (or use Settings → API base URL in the app).

```bash
cd front && pnpm install && pnpm dev
```

## API reference

See [`api.md`](api.md) for routes, payloads, auth rules, rate limiting, and gRPC coverage.

## Development

- `make proto` — regenerate Go code from protos (`buf generate`).
- `make build` — compile to `bin/urlo`.
- `go test ./...` — run Go tests.
