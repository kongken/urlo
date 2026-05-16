# urlo API

URL shortener service. Exposes both an HTTP/JSON API (port `8080`) and a
gRPC API (port `9090`); both are backed by the same `urlo.v1.UrlService`.

- **Base URL (HTTP)**: `http://<host>:8080`
- **gRPC service**: `urlo.v1.UrlService` (`pkg/proto/urlo/v1/service.proto`)
- **Content type**: `application/json` for all HTTP request/response bodies

## Contents

| Doc | Covers |
|-----|--------|
| [Conventions](./conventions.md) | `ShortLink` / `ClickEvent` shapes, error format, gRPC→HTTP status mapping |
| [Auth](./auth.md) | Authentication model + `/api/v1/auth/*` endpoints |
| [URLs](./urls.md) | `/api/v1/urls*` — list, shorten, resolve, stats, update, status, availability, delete |
| [Clicks & analytics](./clicks.md) | `/clicks` event log + `/analytics` aggregations |
| [Redirect & health](./redirect.md) | `GET /:code` public redirect, `GET /health` |
| [gRPC](./grpc.md) | gRPC surface on port `9090` |

## Quick reference

| Method | Path | Description |
|--------|------|-------------|
| `GET`  | `/health` | Liveness probe |
| `POST` | `/api/v1/auth/google` | Exchange Google ID token for session |
| `POST` | `/api/v1/auth/logout` | Clear session |
| `GET`  | `/api/v1/auth/me` | Current user |
| `GET`  | `/api/v1/urls` | List my links |
| `POST` | `/api/v1/urls` | Shorten |
| `GET`  | `/api/v1/urls/availability` | Check custom code availability |
| `GET`  | `/api/v1/urls/:code` | Resolve (counts visit) |
| `GET`  | `/api/v1/urls/:code/lookup` | Resolve alias |
| `GET`  | `/api/v1/urls/:code/stats` | Stats (no visit increment) |
| `PATCH`| `/api/v1/urls/:code` | Update link |
| `GET`  | `/api/v1/urls/:code/status` | Read enable/disable status |
| `PATCH`| `/api/v1/urls/:code/status` | Toggle enable/disable |
| `DELETE`| `/api/v1/urls/:code` | Delete |
| `GET`  | `/api/v1/urls/:code/clicks` | List click events |
| `GET`  | `/api/v1/urls/:code/analytics` | Aggregated analytics |
| `GET`  | `/:code` | Public redirect |
