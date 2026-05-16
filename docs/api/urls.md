[← API Docs](./README.md)

# URL endpoints

See [auth.md](./auth.md) for ownership/auth rules and [conventions.md](./conventions.md) for the `ShortLink` shape and error format.

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
| `code_length` | integer | no       | Length of auto-generated code; must be in `[6, 32]`. `0` (default) uses the server-configured length. Ignored when `custom_code` is set. |

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

**Errors**: `400` (invalid body / `long_url` / `custom_code` / `ttl_seconds` /
`code_length`), `409` (`custom_code` already used), `429` (rate-limited).

---

### `GET /api/v1/urls/:code` — Resolve

Look up a short link by code. **Increments `visit_count` by 1** on success.
Use this when you need the JSON details (the redirect endpoint also
counts visits).

```bash
curl http://localhost:8080/api/v1/urls/aB3xQ7
```

**200 OK** — returns a `ShortLink`.
**Errors**: `404` (not found / expired / disabled).

---

### `GET /api/v1/urls/:code/lookup` — Lookup

Alias of `Resolve` for clients that prefer an explicit lookup route.
Same response and side effects as `GET /api/v1/urls/:code` (includes
`visit_count` increment).

**200 OK** — returns a `ShortLink`.
**Errors**: `404` (not found / expired / disabled).

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

### `PATCH /api/v1/urls/:code` — Update link

Update mutable fields of a short link. Same ownership rules as `Delete`.

**Request body**

| Field         | Type    | Required | Notes |
|---------------|---------|----------|-------|
| `long_url`    | string  | no       | New destination URL |
| `ttl_seconds` | integer | no       | `0` clears expiration, `>0` resets expiration from now |

At least one field must be provided.

```bash
curl -X PATCH --cookie 'urlo_session=…' \
  -H 'Content-Type: application/json' \
  -d '{"long_url":"https://example.com/new-target"}' \
  http://localhost:8080/api/v1/urls/aB3xQ7
```

**200 OK** — returns updated `ShortLink`.
**Errors**: `400` (invalid body / invalid URL / no fields),
`403` (not owner), `404` (not found).

---

### `PATCH /api/v1/urls/:code/status` — Enable / disable link

Toggle link availability without deleting it. Disabled links resolve as `404`.
Same ownership rules as `Delete`.

**Request body**

| Field       | Type    | Required | Notes |
|-------------|---------|----------|-------|
| `disabled`  | boolean | yes      | `true` disables, `false` enables |
| `reason`    | string  | no       | Optional reason (stored for admin/debug context) |

```bash
curl -X PATCH --cookie 'urlo_session=…' \
  -H 'Content-Type: application/json' \
  -d '{"disabled":true,"reason":"abuse"}' \
  http://localhost:8080/api/v1/urls/aB3xQ7/status
```

**200 OK**
```json
{ "code": "aB3xQ7", "disabled": true, "reason": "abuse" }
```

**Errors**: `400` (invalid body), `403` (not owner), `404` (not found).

---

### `GET /api/v1/urls/:code/status` — Read current enable/disable status

Return the persisted status for a short link. Same ownership rules as
`GetStats`.

```bash
curl --cookie 'urlo_session=…' \
  http://localhost:8080/api/v1/urls/aB3xQ7/status
```

**200 OK**
```json
{ "code": "aB3xQ7", "disabled": true, "reason": "abuse" }
```

**Errors**: `403` (not owner), `404` (not found).

---

### `GET /api/v1/urls/availability` — Check custom code availability

Check whether a custom code is currently free to use.

**Query parameters**

| Param   | Type   | Required | Notes |
|---------|--------|----------|-------|
| `code`  | string | yes      | Must match code rules (`[A-Za-z0-9]{1,32}`) |

```bash
curl 'http://localhost:8080/api/v1/urls/availability?code=launch'
```

**200 OK**
```json
{ "code": "launch", "available": true }
```

**Errors**: `400` (missing/invalid code).

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
