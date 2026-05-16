[← API Docs](./README.md)

# Authentication

Google ID-token login is optional. When `auth.google.client_id` is unset
on the server, all `/api/v1/auth/*` routes return **503** and every link
is anonymous; otherwise the API issues an HMAC-signed JWT in an HTTP-only
session cookie (default name `urlo_session`).

- Endpoints that **require** auth: `GET /api/v1/urls`. Missing/invalid
  cookie → **401**.
- Endpoints with **optional** ownership: `POST /api/v1/urls`,
  `GET /api/v1/urls/:code/stats`, `GET /api/v1/urls/:code/clicks`,
  `GET /api/v1/urls/:code/analytics`, `PATCH /api/v1/urls/:code`,
  `PATCH /api/v1/urls/:code/status`, `DELETE /api/v1/urls/:code`.
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
