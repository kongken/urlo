[← API Docs](./README.md)

# gRPC

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
