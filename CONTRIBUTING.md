# Contributing

The repository holds six modules. Run every command in each of them.

| Module | Holds | Dependencies |
|---|---|---|
| `.` | `gashe`, `gashe/backend/memory`, `gashe/backend/nearcache`, `gashe/backendtest`, `gashe/httpcache`, `gashe/codec/raw`, `gashe/compress/gzip` | none |
| `backend/redis/` | `gashe/backend/redis` | go-redis v9 |
| `codec/msgpack/` | `gashe/codec/msgpack` | vmihailenco/msgpack v5 |
| `compress/zstd/` | `gashe/compress/zstd` | klauspost/compress |
| `examples/` | runnable examples, never released | go-redis v9 |
| `benchmarks/` | benchmarks, `just bench`, never released | the gashe modules |

The tests of `backend/redis` against a real server read its address from `GASHE_REDIS_ADDR` and are
skipped without it, so `go test` needs no Docker. `just redis test` starts a server of
`backend/redis/compose.yaml` in Docker and runs them against the local code of the repository
through `go.work`; `just redis all` runs them on Redis 7.0 and Valkey 8, and `just redis cluster`
on a Redis Cluster of three masters, where keys of one call land on different nodes.
`just redis down` stops the servers.

The modules support Go 1.25 and newer: the two releases Go supports, and the one before them.

Tasks run with [just](https://just.systems) and go through every module; `just` lists them:

```bash
just check          # before a commit: golangci-lint, tests with -race, govulncheck
just release-check  # before a tag: check, the examples, Redis, Valkey and a Cluster in Docker, gorelease
just fmt            # format with the formatters of .golangci.yml
just bench -count=5 # the benchmarks; update the tables in docs/en/internals/benchmarks.md from them
just redis all      # the Redis tests on Redis 7.0 and Valkey 8, in Docker
just redis cluster  # the Cluster tests on three masters, in Docker
REDIS_IMAGE=docker.dragonflydb.io/dragonflydb/dragonfly just redis test redis -v  # any other server
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
tag the root first (`v0.5.0`), raise the version the other modules require, and tag the modules
that changed with their directory in front (`backend/redis/v0.4.0`). The latest tags are `v0.5.0`,
`backend/redis/v0.4.0`, `codec/msgpack/v0.3.0` and `compress/zstd/v0.1.0`. Copy the section into
the GitHub release of the tag.

Domain terms are defined in [docs/GLOSSARY.md](docs/GLOSSARY.md). The API reference is the doc
comments, shown on pkg.go.dev: every exported name needs one, and the linters check it.

The documentation site is built with [Zensical](https://zensical.org) from `docs/en/` and
published to GitHub Pages on every push to `main`. Its tools are Python packages listed in
`docs/pyproject.toml` and pinned in `docs/uv.lock`; [uv](https://docs.astral.sh/uv/) installs them,
and the recipes run them through `uv run --project docs`:

```bash
uv sync --project docs # install the tools into docs/.venv; the recipes also do it on their own
just docs build      # build the site into site/; a broken link or anchor fails it
just docs serve      # preview it at http://localhost:8000/gashe/
just docs reference  # write the API reference from the doc comments, after an API change
just docs links      # external links, with lychee in Docker; CI checks them weekly
```

The pages include the code of the examples from `examples/` with `--8<--`: the part between
`// --8<-- [start:api]` and `// --8<-- [end:api]`, so a page shows the API, not the setup. An
example prints nothing: a comment at each call says what it returns, checked against a real run.
Responses of curl and what the backend holds are in `docs/snippets/`. Headings carry fixed ids
(`{ #id }`), so links keep working when a heading is reworded.

The documentation is in English only.
