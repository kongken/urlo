[Back to API Docs](./README.md)

# URL expansion

### `POST /api/v1/expand` — Restore a third-party URL

Follow public HTTP redirects and return the final destination without
creating or reading a urlo short-link record. The endpoint is anonymous.
When API rate limiting is enabled, it is limited under the `expand` scope.

**Request body**

| Field | Type | Required | Notes |
|-------|------|----------|-------|
| `url` | string | yes | Absolute `http` or `https` URL |

```bash
curl -X POST http://localhost:8080/api/v1/expand \
  -H 'Content-Type: application/json' \
  -d '{"url":"https://bit.ly/example"}'
```

**200 OK**

```json
{
  "input_url": "https://bit.ly/example",
  "final_url": "https://example.com/article?id=123",
  "status_code": 200,
  "redirect_count": 2,
  "redirects": [
    {
      "url": "https://bit.ly/example",
      "status_code": 301,
      "location": "https://example.com/step-1"
    },
    {
      "url": "https://example.com/step-1",
      "status_code": 302,
      "location": "https://example.com/article?id=123"
    }
  ]
}
```

`status_code` is the final HTTP response status. A final `4xx` or `5xx`
response is still returned as a successful expansion when the destination
was reached. The supported redirect statuses are `301`, `302`, `303`, `307`,
and `308`.

The server applies a total request timeout and a maximum redirect count from
the `expander` configuration. It only contacts publicly routable IP
addresses, checks every redirect target, and does not forward caller cookies
or authorization headers.

**Errors**: `400` (invalid URL, malformed redirect, redirect loop, or too many
redirects), `403` (target blocked by network policy), `429` (rate-limited),
`503` (upstream unavailable), `504` (timeout).

This endpoint follows server-side HTTP redirects only. It does not execute
JavaScript, parse HTML `meta refresh`, bypass login pages, or defeat bot and
anti-automation checks.
