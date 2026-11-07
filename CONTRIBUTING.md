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
skipped without it, so `go test` needs no Docker. `just redis-test` starts a server of
`backend/redis/docker-compose.yaml` in Docker and runs them against the local code of the repository
through `go.work`; `just redis-all` runs them on Redis 7.0 and Valkey 8, and `just redis-cluster`
on a Redis Cluster of three masters, where keys of one call land on different nodes.
`just redis-down` stops the servers.

The modules support Go 1.25 and newer: the two releases Go supports, and the one before them.

Tasks run with [just](https://just.systems) and go through every module; `just` lists them:

```bash
just check-commit   # before a commit: golangci-lint, typos, tombi, actionlint, rumdl, tests with -race, govulncheck
just check-release  # before a tag: check-commit, the examples, Redis, Valkey and a Cluster in Docker, the site, gorelease
just golangci-fmt   # format with the formatters of .golangci.yml
just prek-install   # install the git hooks of prek.toml, which run the linters on the changed files
just bench -count=5 # the benchmarks; update the tables in docs/en/internals/benchmarks.md from them
just redis-all      # the Redis tests on Redis 7.0 and Valkey 8, in Docker
just redis-cluster  # the Cluster tests on three masters, in Docker
REDIS_IMAGE=docker.dragonflydb.io/dragonflydb/dragonfly just redis-test redis -v  # any other server
```

`govulncheck` and `gorelease` run through `go run`, golangci-lint and actionlint in Docker, and
typos, tombi and prek through uv, all at the versions pinned in the `justfile` and `docs/uv.lock`, so
nothing has to be installed but Go, Docker, uv and just. `govulncheck` also reports the standard
library of the Go that runs it: keep the local Go on its latest patch release. `just gorelease` compares
the API with the last tag and needs a clean working tree.

`backend/redis`, `codec/msgpack` and `compress/zstd` require a released version of the root module.
To change the root and another module together, they need a workspace; `just` creates `go.work` on
its first run, so every module builds against the local code of the others. `go.work` is not
committed, and `govulncheck`, `gorelease` and `go mod tidy` run without it, on the released versions.

The examples and benchmarks modules are never published, so their `go.mod` replaces the gashe
modules with the local copies and always builds against the code of the repository.

To release, replace `Unreleased` in the top section of `CHANGELOG.md` with the version and the
date and merge it. Then, on `main`, run `just check-release`, tag each module that changed with
`just commitizen-bump`, which takes the next version from its `cz.toml` and the commits since its
last tag (`just commitizen-bump`, `just commitizen-bump backend/redis`), and push the tags with
`git push origin <tag>...`. The root goes first when another module needs its new version.
commitizen counts the commits of the whole repository, so check what it proposes; `just
commitizen-bump compress/zstd PATCH` takes a chosen increment. A tag gets its GitHub release from
CI.

Commit messages follow [Conventional Commits](https://www.conventionalcommits.org) (`fix:`,
`feat:`, `docs:`, `build!:`); the `commit-msg` hook that `just prek-install` installs checks them.

Changes reach `main` only by a squash-merged pull request whose checks pass. A pull request that users
see adds an entry under `Unreleased` in `CHANGELOG.md`.

Domain terms are defined in [docs/GLOSSARY.md](docs/GLOSSARY.md). The API reference is the doc
comments, shown on pkg.go.dev: every exported name needs one, and the linters check it.

The documentation site is built with [Zensical](https://zensical.org) from `docs/en/` and
published to GitHub Pages on every push to `main`. Its tools are Python packages listed in
`docs/pyproject.toml` and pinned in `docs/uv.lock`; [uv](https://docs.astral.sh/uv/) installs them,
and the recipes run them through `uv run --project docs`:

```bash
uv sync --project docs # install the tools into docs/.venv; the recipes also do it on their own
just rumdl-lint        # lint the Markdown of the pages with rumdl; `just rumdl-fmt` fixes it
just zensical-build    # build the site into docs/site/; a broken link or anchor fails it
just zensical-serve    # preview it at http://localhost:8000/gashe/
just gomarkdoc         # write docs/en/reference/ from the doc comments; the builds run it, it is not committed
just lychee            # external links, with lychee in Docker; CI checks them weekly
```

The pages include the code of the examples from `examples/` with `--8<--`: the part between
`// --8<-- [start:api]` and `// --8<-- [end:api]`, so a page shows the API, not the setup. An
example prints nothing: a comment at each call says what it returns, checked against a real run.
Responses of curl and what the backend holds are in `docs/snippets/`. Headings carry fixed ids
(`{ #id }`), so links keep working when a heading is reworded.

The documentation is in English only.
