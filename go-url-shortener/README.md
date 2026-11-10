# go-url-shortener

A small URL shortener written with nothing but the Go standard library. It creates base62 short IDs, redirects, counts clicks, supports link expiry, rate-limits clients per IP, and stores links either in memory or in a JSON file behind a `Store` interface.

## Features

- `net/http` server using Go 1.22+ method/wildcard routing (`GET /{id}`)
- Base62 IDs generated from a sequence counter (`internal/base62`)
- `Store` interface with an in-memory implementation and a JSON-file implementation (atomic write via temp file + rename)
- Click counting (expired links are not counted)
- Optional expiry per link (`ttl_seconds`), expired links answer `410 Gone`
- Token-bucket rate limiting per client IP with `Retry-After` (`429`)
- URL validation (absolute http/https only, 2048 chars max)
- Tests with `httptest`, fake clocks, and concurrency checks

## Quick start

```sh
go run ./cmd/shortener -addr :8080 -base-url http://localhost:8080 -data links.json
```

Flags: `-addr`, `-base-url`, `-data` (omit for in-memory only), `-rate` (requests/s per IP, default 5), `-burst` (default 20).

```sh
curl -X POST localhost:8080/api/links -d '{"url":"https://github.com/nidhalom","ttl_seconds":3600}'
curl -i localhost:8080/1001          # 302 to the target
curl localhost:8080/api/links/1001   # stats
```

Real output from a run:

```
$ curl -X POST localhost:18080/api/links -d '{"url":"https://example.com/docs"}'
$ curl -i -o /dev/null -w "%{http_code} %{redirect_url}\n" localhost:18080/1002
302 https://example.com/docs
$ curl localhost:18080/api/links/1002     # after two redirects
{"id":"1002","url":"https://example.com/docs","created_at":"2026-10-18T00:01:22.4166748Z","clicks":2,"short_url":"http://localhost:18080/1002","expired":false}
$ curl -X POST localhost:18080/api/links -d '{"url":"ftp://x"}'
{"error":"url must be an absolute http(s) URL"}
```

## API

| Method | Path | Description |
| --- | --- | --- |
| `POST` | `/api/links` | Body `{"url": "...", "ttl_seconds": 0}`. Returns `201` with the link and `short_url`. |
| `GET` | `/api/links/{id}` | Link stats: clicks, expiry, `expired` flag. |
| `GET` | `/{id}` | `302` redirect and click increment; `404` unknown, `410` expired. |
| `GET` | `/healthz` | Liveness. |

## Project structure

```
cmd/shortener/        entry point (flags, graceful shutdown)
internal/base62/      integer <-> base62 string
internal/store/       Store interface, Memory and JSON-file implementations
internal/ratelimit/   per-key token bucket
internal/server/      HTTP handlers and middleware
```

## How it works

Each created link gets the next number from a counter (offset by 62^3 so IDs are at least four characters), encoded in base62. `Store.Hit` resolves a link, rejects it if expired, and increments the counter under a mutex. The JSON store keeps everything in memory and rewrites the file atomically after each change. The limiter keys on the TCP peer address; `X-Forwarded-For` is deliberately ignored because clients can forge it.

## Testing

```sh
go test ./...
```

## Limitations

- IDs are sequential, so they are guessable and enumerable. Do not use for secret links.
- The JSON store rewrites the whole file on every click, which is fine for small data sets only.
- Rate limiter state is per process and in memory.
- Behind a reverse proxy all clients share the proxy IP unless you add a trusted-proxy key function.

## License

MIT, see [LICENSE](LICENSE).
