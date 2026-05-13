# urlo

[![Go](https://github.com/kongken/urlo/actions/workflows/go.yml/badge.svg)](https://github.com/kongken/urlo/actions/workflows/go.yml)
[![codecov](https://codecov.io/gh/kongken/urlo/branch/main/graph/badge.svg)](https://codecov.io/gh/kongken/urlo)

`urlo` is a Go-based URL shortener with a REST API, gRPC service, and a React frontend for creating and managing links.

## Implemented Features

- Shorten long URLs with generated codes or custom aliases
- Configurable generated code length from 6 to 32 characters
- Optional expiration via `ttl_seconds`
- Public redirect endpoint and JSON lookup endpoints
- Visit counter on resolve and redirect
- Per-user link ownership when Google login is enabled
- My Links dashboard for listing, filtering, refreshing, editing, disabling, and deleting links
- Custom-code availability check before creating links
- Link status management with enable/disable reason
- Click-event logging with referrer, device, browser, OS, language, and bot detection
- Aggregated analytics by day, country, and referrer
- QR code generation in the landing page, dashboard, and analytics page
- Pluggable storage backends: in-memory and S3
- Optional per-IP shorten rate limiting backed by Redis
- Optional click-event recording backed by Redis Streams
- HTTP/JSON API, gRPC API, and Connect-generated clients

## Architecture

- Backend entrypoint: `cmd/urlo`
- HTTP routes: `internal/http`
- Core URL service and store interface: `internal/url`
- Auth and session handling: `internal/auth`
- Click logging and analytics helpers: `internal/clicks`
- Frontend app: `front`
- Protobuf definitions: `proto/urlo/v1`

## API Surface

HTTP endpoints include:

- `POST /api/v1/urls` create short links
- `GET /api/v1/urls` list the authenticated user's links
- `GET /api/v1/urls/:code` resolve a link and increment visits
- `GET /api/v1/urls/:code/lookup` explicit lookup alias for resolve
- `GET /api/v1/urls/:code/stats` fetch stats without incrementing visits
- `GET /api/v1/urls/:code/clicks` list recorded click events
- `GET /api/v1/urls/:code/analytics` fetch aggregated analytics
- `PATCH /api/v1/urls/:code` update destination or TTL
- `GET /api/v1/urls/:code/status` read disabled state
- `PATCH /api/v1/urls/:code/status` enable or disable a link
- `GET /api/v1/urls/availability` check custom-code availability
- `DELETE /api/v1/urls/:code` delete a link
- `POST /api/v1/auth/google`, `POST /api/v1/auth/logout`, `GET /api/v1/auth/me`
- `GET /:code` public redirect

See `api.md` for request and response details.

## Configuration

The backend supports these runtime features through `config.yaml`:

- `base_url` for generated short URLs
- `code_length` for generated code size
- `storage.driver` with `memory` or `s3`
- `rate_limit.enabled` and Redis-backed shorten limiting
- `auth.google.client_id` and session-cookie auth
- `clicks.driver` with `none` or `redis_stream`

## Local Development

### Backend

```bash
make run
```

Useful commands:

```bash
make build
go test ./...
make proto
```

The default local config serves HTTP on `:8080`, gRPC on `:9090`, uses in-memory storage, and keeps auth and click logging disabled.

### Frontend

```bash
cd front
pnpm install
pnpm dev
```

By default the frontend talks to the current origin. You can override the API base URL with `VITE_API_BASE_URL` or from the settings page in the browser.

## Frontend Capabilities

The React app currently includes:

- Landing page with URL creation, optional custom code, and code-length toggle
- Availability check feedback for custom aliases
- Dashboard for signed-in links or locally stored anonymous links
- Inline link refresh, edit, delete, QR view, analytics navigation, and status toggling
- Analytics page with click totals, unique visitor estimate from hashed IPs, referrer breakdown, country breakdown, device/browser summaries, and QR export
- Settings page for API base URL override and clearing locally stored links

## Notes

- Ownership enforcement exists in the HTTP layer. The gRPC service itself is unauthenticated.
- Anonymous links are stored in browser local storage by the frontend so they remain manageable without login.
- Disabled links return `404` on resolve and redirect.
