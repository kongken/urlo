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

### `ClickEvent` object

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

The rate-limit middleware uses a separate `error: "rate_limited"` body and
sets a `Retry-After` header (seconds until the window resets).

### Authentication

Google ID-token login is optional. When `auth.google.client_id` is unset
on the server, all `/api/v1/auth/*` routes return **503** and every link
is anonymous; otherwise the API issues an HMAC-signed JWT in an HTTP-only
session cookie (default name `urlo_session`).

- Endpoints that **require** auth: `GET /api/v1/urls`. Missing/invalid
  cookie → **401**.
- Endpoints with **optional** ownership: `POST /api/v1/urls`,
  `GET /api/v1/urls/:code/stats`, `GET /api/v1/urls/:code/clicks`,
  `DELETE /api/v1/urls/:code`.
  - Anonymous request to a link with no owner: allowed.
  - Authenticated request matching the link's owner: allowed.
  - Mismatch (owner set, caller is anonymous or different user): **403**.
- Anonymous `POST /api/v1/urls` creates an owner-less link that anyone
  can view, edit, or delete by code.

`User` payload returned by `/auth/me` and `/auth/google`:

| Field       | Type   | Description           |
|-------------|--------|-----------------------|
| `sub`       | string | Google subject id     |
| `email`     | string | Verified email        |
| `name`      | string | Display name          |
| `picture`   | string | Avatar URL (optional) |

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

### `POST /api/v1/auth/google` — Exchange Google ID token

Verify a Google ID token and start a session. Sets an HTTP-only
`urlo_session` cookie on success.

**Request body**

| Field       | Type   | Required | Notes                                |
|-------------|--------|----------|--------------------------------------|
| `id_token`  | string | yes      | ID token obtained from Google Sign-In |

```bash
curl -X POST http://localhost:8080/api/v1/auth/google \
  -H 'Content-Type: application/json' \
  -d '{"id_token":"<google_id_token>"}'
```

**200 OK**
```json
{ "user": { "sub": "1234…", "email": "you@example.com", "name": "You" } }
```

**Errors**: `400` (missing `id_token`), `401` (invalid token),
`503` (auth disabled on the server).

---

### `POST /api/v1/auth/logout` — Clear session

Clears the session cookie. **204 No Content**. Returns **503** when auth
is disabled on the server.

---

### `GET /api/v1/auth/me` — Current user

Returns the user attached to the current session.

**200 OK**
```json
{ "user": { "sub": "1234…", "email": "you@example.com", "name": "You" } }
```

**Errors**: `401` (no/invalid session), `503` (auth disabled).

---

### `GET /api/v1/urls` — List my links

Lists short links owned by the authenticated caller. Expired links are
omitted.

```bash
curl --cookie 'urlo_session=…' http://localhost:8080/api/v1/urls
```

**200 OK**
```json
{ "links": [ { "code": "aB3xQ7", "long_url": "…", … } ] }
```

**Errors**: `401` (login required).

---

### `POST /api/v1/urls` — Shorten

Create a new short link. The server generates a 6-char code unless
`custom_code` is provided. When the caller is authenticated, the new
link is tagged with their `sub` as owner.

Subject to per-IP rate limiting when `rate_limit.enabled = true`.
Limited callers receive **429** with `Retry-After`.

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
`409` (`custom_code` already used), `429` (rate-limited).

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
If the link has an owner, the caller must be authenticated as that owner
or the request fails with **403**.

```bash
curl http://localhost:8080/api/v1/urls/aB3xQ7/stats
```

**200 OK** — returns a `ShortLink`.
**Errors**: `403` (not owner), `404` (not found).

---

### `GET /api/v1/urls/:code/clicks` — ListClicks

Return recent click events for a short link, newest first. Same
ownership rules as `GetStats`. Returns `[]` when click logging is
disabled (`clicks.driver = "none"`).

**Query parameters**

| Param         | Type    | Default | Notes                                       |
|---------------|---------|---------|---------------------------------------------|
| `page_size`   | integer | 50      | Capped at 500                               |
| `page_token`  | string  | —       | Opaque cursor from a previous response      |

```bash
curl --cookie 'urlo_session=…' \
  'http://localhost:8080/api/v1/urls/aB3xQ7/clicks?page_size=20'
```

**200 OK**
```json
{
  "events": [
    {
      "id": "1714521600123-0",
      "code": "aB3xQ7",
      "ts": "2026-05-01T16:00:00Z",
      "ip_hash": "9c1185a5c5e9fc54",
      "referrer": "https://www.google.com/",
      "referrer_host": "www.google.com",
      "user_agent": "Mozilla/5.0 …",
      "browser": "Chrome",
      "os": "macOS",
      "device": "desktop",
      "lang": "en-US",
      "is_bot": false
    }
  ],
  "next_page_token": "1714521590000-0"
}
```

Pass `next_page_token` back as `page_token` to fetch older events; an
empty string means no more pages.

**Errors**: `403` (not owner), `404` (not found).

---

### `DELETE /api/v1/urls/:code` — Delete

Remove a short link. If the link has an owner, the caller must be
authenticated as that owner.

```bash
curl -X DELETE --cookie 'urlo_session=…' \
  http://localhost:8080/api/v1/urls/aB3xQ7
```

**204 No Content** on success.
**Errors**: `403` (not owner), `404` (not found).

---

### `GET /:code` — Redirect

Public short-link entry point. **Increments `visit_count` by 1** and, if
click logging is enabled, asynchronously records a `ClickEvent`.

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

| RPC          | Request              | Response              |
|--------------|----------------------|-----------------------|
| `Shorten`    | `ShortenRequest`     | `ShortenResponse`     |
| `Resolve`    | `ResolveRequest`     | `ResolveResponse`     |
| `GetStats`   | `GetStatsRequest`    | `GetStatsResponse`    |
| `Delete`     | `DeleteRequest`      | `DeleteResponse`      |
| `ListClicks` | `ListClicksRequest`  | `ListClicksResponse`  |

```bash
grpcurl -plaintext \
  -d '{"long_url":"https://example.com"}' \
  localhost:9090 urlo.v1.UrlService/Shorten

grpcurl -plaintext \
  -d '{"code":"aB3xQ7","page_size":20}' \
  localhost:9090 urlo.v1.UrlService/ListClicks
```

> The gRPC surface is unauthenticated and does **not** enforce ownership
> — auth lives in the HTTP layer. Front it with an authenticating proxy
> if you expose it externally.

Schemas: see `proto/urlo/v1/url.proto` and `proto/urlo/v1/service.proto`.
Generated Go bindings live under `pkg/proto/urlo/v1/`.
