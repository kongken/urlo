[← API Docs](./README.md)

# Clicks & Analytics

See [conventions.md](./conventions.md) for the `ClickEvent` shape and [auth.md](./auth.md) for ownership rules.

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

### `GET /api/v1/urls/:code/analytics` — Aggregated analytics

Return aggregate click metrics derived from recorded click events.
Same ownership rules as `GetStats`.

**Query parameters**

| Param         | Type    | Required | Notes |
|---------------|---------|----------|-------|
| `stats_type`  | string  | yes      | `day`, `country`, or `referer` |
| `from`        | string  | no       | RFC 3339 lower bound (inclusive) |
| `to`          | string  | no       | RFC 3339 upper bound (inclusive) |
| `limit`       | integer | no       | Top-N for `country` and `referer` (default 50) |

```bash
curl --cookie 'urlo_session=…' \
  'http://localhost:8080/api/v1/urls/aB3xQ7/analytics?stats_type=referer&limit=5'
```

**200 OK**
```json
{
  "code": "aB3xQ7",
  "stats_type": "referer",
  "items": [
    { "key": "www.google.com", "count": 123 },
    { "key": "(direct)", "count": 42 }
  ]
}
```

**Errors**: `400` (invalid `stats_type` / bad `from` / bad `to`),
`403` (not owner), `404` (not found).
