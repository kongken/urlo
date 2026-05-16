[← API Docs](./README.md)

# Redirect & health

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
