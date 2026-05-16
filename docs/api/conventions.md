[← API Docs](./README.md)

# Conventions

## `ShortLink` object

Every successful endpoint that returns a link uses the following shape:

| Field         | Type     | Description                                                         |
|---------------|----------|---------------------------------------------------------------------|
| `code`        | string   | Short code (`[A-Za-z0-9]`, 1–32 chars)                              |
| `long_url`    | string   | Original long URL                                                   |
| `short_url`   | string   | Fully-qualified short URL (e.g. `https://urlo.example/abc123`)      |
| `created_at`  | string   | RFC 3339 timestamp                                                  |
| `expires_at`  | string   | RFC 3339 timestamp; omitted when there is no expiration             |
| `visit_count` | integer  | Total successful resolves (incremented by `Resolve` and redirect)   |
| `disabled`    | boolean  | Present when true; disabled links resolve as `404`                  |

## `ClickEvent` object

Returned by the click-log endpoint:

| Field           | Type    | Description                                                    |
|-----------------|---------|----------------------------------------------------------------|
| `id`            | string  | Server-assigned event id (Redis stream id, e.g. `1714…-0`)     |
| `code`          | string  | Short code                                                     |
| `ts`            | string  | RFC 3339 timestamp of the click                                |
| `ip_hash`       | string  | First 16 hex chars of `sha256(ip + salt)`; empty if disabled   |
| `country`       | string  | Country code (empty until GeoIP is wired)                      |
| `city`          | string  | City (empty until GeoIP is wired)                              |
| `referrer`      | string  | Full `Referer` header value                                    |
| `referrer_host` | string  | Lowercase hostname extracted from `referrer`                   |
| `user_agent`    | string  | Raw `User-Agent` header                                        |
| `browser`       | string  | `Chrome` / `Firefox` / `Safari` / `Edge` / `Opera` / `Bot` / `Other` |
| `os`            | string  | `Windows` / `macOS` / `iOS` / `Android` / `Linux` / `Other`    |
| `device`        | string  | `desktop` / `mobile` / `tablet` / `bot` / `other`              |
| `lang`          | string  | First tag of `Accept-Language`                                 |
| `is_bot`        | boolean | True if the UA matches a known bot signature                   |

## Error response

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

The rate-limit middleware uses a separate `error: "rate_limited"` body and
sets a `Retry-After` header (seconds until the window resets).
