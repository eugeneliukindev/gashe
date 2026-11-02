# Contributing

The repository holds four modules. Run every command in each of them.

| Module | Holds | Dependencies |
|---|---|---|
| `.` | `gashe`, `gashe/backend/memory`, `gashe/backend/nearcache`, `gashe/backendtest`, `gashe/httpcache` | none |
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

Tasks run with [just](https://just.systems) and go through every module; `just` lists them:

```bash
just check          # before a commit: golangci-lint, tests with -race, govulncheck
just release-check  # before a tag: check, tests on Go 1.23 through goenv, Redis in Docker, gorelease
just fmt            # format with the formatters of .golangci.yml
just integration valkey/valkey:8  # the Redis tests against another server
```

`govulncheck` and `gorelease` run through `go run` at the versions pinned in the `justfile`, so
nothing has to be installed but Go, golangci-lint and just. `govulncheck` also reports the standard
library of the Go that runs it: keep the local Go on its latest patch release. `just api` compares
the API with the last tag and needs a clean working tree.

`backend/redis` and `codec/msgpack` require a released version of the root module. To change the
root and another module together, create a workspace once; `go.work` is not committed:

```bash
go work init . ./backend/redis ./backend/redis/integration ./codec/msgpack
```

The integration module is never published, so its `go.mod` replaces the gashe modules with the
local copies and always tests the code of the repository.

To release, replace `Unreleased` in the top section of `CHANGELOG.md` with the date, then
tag the root first (`v0.2.0`), raise the version the other modules require, and tag them with
their directory in front: `backend/redis/v0.2.0`, `codec/msgpack/v0.2.0`. Copy the section into
the GitHub release of the tag.

Domain terms are defined in [GLOSSARY.md](GLOSSARY.md). `REFERENCE.md` lists the public API; a test
fails when an exported name is missing from it.
