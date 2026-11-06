# Contributing

The repository holds six modules. Run every command in each of them.

| Module | Holds | Dependencies |
|---|---|---|
| `.` | `gashe`, `gashe/backend/memory`, `gashe/backend/nearcache`, `gashe/backendtest`, `gashe/httpcache` | none |
| `backend/redis/` | `gashe/backend/redis` | go-redis v9 |
| `codec/msgpack/` | `gashe/codec/msgpack` | vmihailenco/msgpack v5 |
| `compress/zstd/` | `gashe/compress/zstd` | klauspost/compress |
| `examples/` | runnable examples, never released | go-redis v9 |
| `benchmarks/` | benchmarks, `just bench`, never released | the gashe modules |

The tests of `backend/redis` against a real server read its address from `GASHE_REDIS_ADDR` and are
skipped without it, so `go test` needs no Docker. `just integration` starts a server in Docker,
runs them against the local code of the repository through `go.work`, and removes the
server; `just integration-all` runs them on Redis 7.0 and Valkey 8, and `just integration-cluster`
on a Redis Cluster of three masters, where keys of one call land on different nodes.

The modules support Go 1.23 and newer, five releases at the moment. Tests that need
`testing/synctest` are behind `//go:build go1.25`, so on Go 1.23 and 1.24 they are skipped and the
rest still run.

Tasks run with [just](https://just.systems) and go through every module; `just` lists them:

```bash
just check          # before a commit: golangci-lint, tests with -race, govulncheck
just release-check  # before a tag: check, tests on Go 1.23 through goenv, Redis and a Cluster in Docker, gorelease
just fmt            # format with the formatters of .golangci.yml
just bench -count=5 # the benchmarks; update the tables in the README from them
just integration-all               # the Redis tests on Redis 7.0 and Valkey 8, in Docker
just integration-cluster           # the Cluster tests on three masters, in Docker
just integration docker.dragonflydb.io/dragonflydb/dragonfly -v  # any other server
```

`govulncheck` and `gorelease` run through `go run` at the versions pinned in the `justfile`, so
nothing has to be installed but Go, golangci-lint and just. `govulncheck` also reports the standard
library of the Go that runs it: keep the local Go on its latest patch release. `just api` compares
the API with the last tag and needs a clean working tree.

`backend/redis`, `codec/msgpack` and `compress/zstd` require a released version of the root module.
To change the root and another module together, they need a workspace; `just` creates `go.work` on
its first run, so every module builds against the local code of the others. `go.work` is not
committed, and `govulncheck`, `gorelease` and `go mod tidy` run without it, on the released versions.

The examples and benchmarks modules are never published, so their `go.mod` replaces the gashe
modules with the local copies and always builds against the code of the repository.

To release, replace `Unreleased` in the top section of `CHANGELOG.md` with the date, then
tag the root first (`v0.2.0`), raise the version the other modules require, and tag them with
their directory in front: `backend/redis/v0.2.0`, `codec/msgpack/v0.2.0`, `compress/zstd/v0.2.0`.
Copy the section into the GitHub release of the tag.

Domain terms are defined in [docs/GLOSSARY.md](docs/GLOSSARY.md). The API reference is the doc
comments, shown on pkg.go.dev: every exported name needs one, and the linters check it.
