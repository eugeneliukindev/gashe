# Contributing

The repository holds four modules. Run every command in each of them.

| Module | Holds | Dependencies |
|---|---|---|
| `.` | `gashe`, `gashe/backend/memory`, `gashe/backend/nearcache`, `gashe/backendtest` | none |
| `backend/redis/` | `gashe/backend/redis` | go-redis v9 |
| `backend/redis/integration/` | tests of `gashe/backend/redis` against a real Redis | testcontainers-go, Docker |
| `codec/msgpack/` | `gashe/codec/msgpack` | vmihailenco/msgpack v5 |

The integration module is separate so that testcontainers never reaches users of `gashe/backend/redis`.
Its tests start `redis:7.0.0` in Docker by themselves and fail if Docker is not available. Set
`GASHE_REDIS_IMAGE` to run them against another server, for example
`GASHE_REDIS_IMAGE=valkey/valkey:8 go test ./...`.

The modules support Go 1.23 and newer, five releases at the moment. Tests that need
`testing/synctest` are behind `//go:build go1.25`, so on Go 1.23 and 1.24 they are skipped and the
rest still run. The integration module needs Go 1.25, as testcontainers does.

```bash
go test -race ./...  # Go 1.25+ runs every test, fake-time ones included
golangci-lint run    # golangci-lint v2, config in .golangci.yml
GOENV_VERSION=1.23.12 GOTOOLCHAIN=local go test -race ./...  # the oldest supported Go
```

`backend/redis` and `codec/msgpack` require a released version of the root module. To change the
root and another module together, create a workspace once; `go.work` is not committed:

```bash
go work init . ./backend/redis ./backend/redis/integration ./codec/msgpack
```

The integration module is never published, so its `go.mod` replaces the gashe modules with the
local copies and always tests the code of the repository.

To release, tag the root first (`v0.2.0`), then raise the version the other modules require, and
tag them with their directory in front: `backend/redis/v0.2.0`, `codec/msgpack/v0.2.0`.

Domain terms are defined in [GLOSSARY.md](GLOSSARY.md). `REFERENCE.md` lists the public API; a test
fails when an exported name is missing from it.
