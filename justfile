# Development tasks for gashe: `just` lists them, `just check` runs before a commit and
# `just release-check` before a tag.

set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

# The documentation site: build, serve, reference, links.
mod docs

# The Redis backend against Redis, Valkey and a Cluster in Docker.
mod redis 'backend/redis'

# Every Go module of the repository, and the ones users import, released with their own tags.
modules := ". backend/redis codec/msgpack compress/zstd examples benchmarks"
released := ". backend/redis codec/msgpack compress/zstd"

# Tools run with go run at pinned versions; golang.org/x/exp has no tags, so gorelease is a commit.
govulncheck := "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
gorelease := "golang.org/x/exp/cmd/gorelease@v0.0.0-20261014143641-f0251eb4aa3e"

# What an example needs that `just examples` cannot give it: such examples are only built.
needs_service := 'redis/go-redis|jackc/pgx|ListenAndServe|signal\.Notify'

[default]
[private]
list:
    @just --list --list-submodules

# Create go.work, so every module builds against the local code of the others; it is not committed.
[private]
work:
    [ -f go.work ] || { go work init {{ modules }} && go work edit -go=1.25.0 -toolchain=none; }

# Run a command in each directory of a list.
[private]
each dirs +cmd:
    for d in {{ dirs }}; do echo "== $d"; (cd "$d" && "${@:2}"); done

# Format the code with the formatters of .golangci.yml.
[group('lint')]
fmt: (each modules "golangci-lint" "fmt" "./...")

# Run golangci-lint in every module, after checking .golangci.yml.
[group('lint')]
lint: work (each modules "golangci-lint" "run" "./...")
    golangci-lint config verify

# Run the tests of the released modules with the race detector; tests that need Redis are skipped.
[group('test')]
test: work
    go test -race ./... ./backend/redis/... ./codec/msgpack/... ./compress/zstd/...

# Run every example; one that needs Redis, Postgres or serves until stopped is only built by lint.
[group('test')]
[working-directory('examples')]
examples: work
    for e in $(find . -name main.go -exec dirname {} \; | sort); do sed 's#//.*##' "$e"/*.go | grep -qE '{{ needs_service }}' && continue; echo "== example $e"; go run "$e" > /dev/null; done

# Run the benchmarks; flags go to go test, such as `just bench -count=5`.
[group('test')]
[working-directory('benchmarks')]
bench *flags: work
    go test -run '^$' -bench . -benchtime 300ms "$@" .

# Report known vulnerabilities in the code and dependencies the released modules call.
[group('security')]
vuln: (each released "env" "GOWORK=off" "go" "run" govulncheck "./...")

# Tidy go.mod and go.sum of every module.
[group('deps')]
tidy: (each modules "env" "GOWORK=off" "go" "mod" "tidy")

# Compare the API of released modules with their last tags; needs a clean working tree.
[group('release')]
api: (each released "env" "GOWORK=off" "go" "run" gorelease)

# Everything to run before a commit.
[group('all')]
check: lint test vuln

# Everything to run before a release tag.
[group('all')]
release-check: check examples redis::all redis::cluster api
