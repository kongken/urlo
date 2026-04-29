# urlo API

URL shortener service. Exposes both an HTTP/JSON API (port `8080`) and a
gRPC API (port `9090`); both are backed by the same `urlo.v1.UrlService`.

- **Base URL (HTTP)**: `http://<host>:8080`
- **gRPC service**: `urlo.v1.UrlService` (`pkg/proto/urlo/v1/service.proto`)
- **Content type**: `application/json` for all HTTP request/response bodies

## Conventions

### `ShortLink` object

Every successful endpoint that returns a link uses the following shape:

| Field         | Type     | Description                                                         |
|---------------|----------|---------------------------------------------------------------------|
| `code`        | string   | Short code (`[A-Za-z0-9]`, 1–32 chars)                              |
| `long_url`    | string   | Original long URL                                                   |
| `short_url`   | string   | Fully-qualified short URL (e.g. `https://urlo.example/abc123`)      |
| `created_at`  | string   | RFC 3339 timestamp                                                  |
| `expires_at`  | string   | RFC 3339 timestamp; omitted when there is no expiration             |
| `visit_count` | integer  | Total successful resolves (incremented by `Resolve` and redirect)   |

### Error response

All non-2xx responses use:

```json
{ "error": "<grpc_code>", "message": "<human readable>" }
```

`error` is the gRPC status code name (`InvalidArgument`, `NotFound`, …).
HTTP status mapping:

| gRPC code              | HTTP |
|------------------------|------|
| `InvalidArgument`      | 400  |
| `Unauthenticated`      | 401  |
| `PermissionDenied`     | 403  |
| `NotFound`             | 404  |
| `AlreadyExists`        | 409  |
| `FailedPrecondition`   | 412  |
| `ResourceExhausted`    | 429  |
| `DeadlineExceeded`     | 504  |
| `Unavailable`          | 503  |
| anything else          | 500  |

## Endpoints

### `GET /health`

Liveness probe.

```bash
curl http://localhost:8080/health
```

**200 OK**
```json
{ "status": "healthy" }
```

---

### `POST /api/v1/urls` — Shorten

Create a new short link. The server generates a 6-char code unless
`custom_code` is provided.

**Request body**

| Field         | Type    | Required | Notes                                                |
|---------------|---------|----------|------------------------------------------------------|
| `long_url`    | string  | yes      | Must parse as a valid URL                            |
| `custom_code` | string  | no       | `[A-Za-z0-9]{1,32}`; `409` if it already exists      |
| `ttl_seconds` | integer | no       | `>= 0`. `0` (default) means no expiration            |

**Example**
```bash
curl -X POST http://localhost:8080/api/v1/urls \
  -H 'Content-Type: application/json' \
  -d '{"long_url":"https://example.com/foo","ttl_seconds":3600}'
```

**201 Created** — returns a `ShortLink`
```json
{
  "code": "aB3xQ7",
  "long_url": "https://example.com/foo",
  "short_url": "http://localhost:8080/aB3xQ7",
  "created_at": "2026-04-30T01:00:00Z",
  "expires_at": "2026-04-30T02:00:00Z",
  "visit_count": 0
}
```

**Errors**: `400` (invalid body / `long_url` / `custom_code` / `ttl_seconds`),
`409` (`custom_code` already used).

---

### `GET /api/v1/urls/:code` — Resolve

Look up a short link by code. **Increments `visit_count` by 1** on success.
Use this when you need the JSON details (the redirect endpoint also
counts visits).

```bash
curl http://localhost:8080/api/v1/urls/aB3xQ7
```

**200 OK** — returns a `ShortLink`.
**Errors**: `404` (not found or expired).

---

### `GET /api/v1/urls/:code/stats` — GetStats

Same payload as `Resolve`, but does **not** increment `visit_count`.
Use this for dashboards and admin views.

```bash
curl http://localhost:8080/api/v1/urls/aB3xQ7/stats
```

**200 OK** — returns a `ShortLink`.
**Errors**: `404` (not found).

---

### `DELETE /api/v1/urls/:code` — Delete

Remove a short link.

```bash
curl -X DELETE http://localhost:8080/api/v1/urls/aB3xQ7
```

**204 No Content** on success.
**Errors**: `404` (not found).

---

### `GET /:code` — Redirect

Public short-link entry point. **Increments `visit_count` by 1**.

```bash
curl -i http://localhost:8080/aB3xQ7
```

**302 Found** with `Location: <long_url>`.
**Errors**: `404` (not found or expired).

> Reserved paths (`/health`, `/ping`, `/api`) are handled by other
> routes and will not be treated as codes.

---

## gRPC

The same operations are available via gRPC on port `9090`:

| RPC        | Request           | Response           |
|------------|-------------------|--------------------|
| `Shorten`  | `ShortenRequest`  | `ShortenResponse`  |
| `Resolve`  | `ResolveRequest`  | `ResolveResponse`  |
| `GetStats` | `GetStatsRequest` | `GetStatsResponse` |
| `Delete`   | `DeleteRequest`   | `DeleteResponse`   |

```bash
grpcurl -plaintext \
  -d '{"long_url":"https://example.com"}' \
  localhost:9090 urlo.v1.UrlService/Shorten
```

Schemas: see `proto/urlo/v1/url.proto` and `proto/urlo/v1/service.proto`.
Generated Go bindings live under `pkg/proto/urlo/v1/`.
