# Development tasks for gashe: `just` lists them, `just check` runs before a commit and
# `just release-check` before a tag.

set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

# Every Go module of the repository, and the ones users import, released with their own tags.
modules := ". backend/redis codec/msgpack compress/zstd examples benchmarks"
released := ". backend/redis codec/msgpack compress/zstd"

# Tools run with go run at pinned versions; golang.org/x/exp has no tags, so gorelease is a commit.
govulncheck := "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
gorelease := "golang.org/x/exp/cmd/gorelease@v0.0.0-20261014143641-f0251eb4aa3e"
gomarkdoc := "go run github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0"

# Linters as Docker images at pinned versions, the same locally and in CI; the Go caches of the host are reused.
golangci := 'docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -v "$PWD":/src -w /src -v "$(go env GOMODCACHE)":/go/pkg/mod -v "$(go env GOCACHE)":/cache/go -e GOCACHE=/cache/go -v "$HOME/.cache/golangci-lint":/cache/lint -e GOLANGCI_LINT_CACHE=/cache/lint golangci/golangci-lint:v2.15.0'
actionlint := 'docker run --rm -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.12'

# The tools of the documentation site, Python packages pinned in docs/uv.lock.
zensical := "uv run --project docs --locked zensical"
rumdl := "uv run --project docs --locked rumdl"
tombi := "uv run --project docs --locked tombi"

# What an example needs that `just examples` cannot give it: such examples are only built.
needs_service := 'redis/go-redis|jackc/pgx|ListenAndServe|signal\.Notify'

[default]
[private]
list:
    @just --list

# Create go.work, so every module builds against the local code of the others; it is not committed.
[private]
work:
    [ -f go.work ] || { go work init {{ modules }} && go work edit -go=1.25.0 -toolchain=none; }

# Run a command in each directory of a list.
[private]
each dirs +cmd:
    for d in {{ dirs }}; do echo "== $d"; (cd "$d" && "${@:2}"); done

# Run golangci-lint with a command in every module, in Docker.
[private]
golangci cmd: work
    mkdir -p "$(go env GOMODCACHE)" "$(go env GOCACHE)" "$HOME/.cache/golangci-lint"
    {{ golangci }} sh -c 'for d in {{ modules }}; do echo "== $d"; (cd "$d" && golangci-lint {{ cmd }} ./...); done'

# Format the code with the formatters of .golangci.yml.
[group('lint')]
fmt: (golangci "fmt")

# Run golangci-lint in every module, after checking .golangci.yml.
[group('lint')]
lint: (golangci "run")
    {{ golangci }} golangci-lint config verify

# Check the format of the TOML files and lint them against their schemas.
[group('lint')]
toml-lint:
    {{ tombi }} format --check
    {{ tombi }} lint

# Format the TOML files.
[group('lint')]
toml-fmt:
    {{ tombi }} format

# Install the git hooks of prek.toml.
[group('lint')]
hooks:
    uv run --project docs --locked prek install

# Run the git hooks on every file, not only the changed ones.
[group('lint')]
hooks-all:
    uv run --project docs --locked prek run --all-files

# Find typos in the whole repository.
[group('lint')]
typos:
    uv run --project docs --locked typos

# Check the GitHub workflows, their shell scripts with shellcheck included.
[group('lint')]
actionlint:
    {{ actionlint }}

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
check: lint typos actionlint toml-lint docs-lint test vuln

# Everything to run before a release tag.
[group('all')]
release-check: check examples redis-all redis-cluster api

# Run the Redis tests against redis or valkey in Docker; flags go to go test: `just redis-test valkey -run Lock`.
[group('redis')]
[working-directory('backend/redis')]
redis-test server="redis" *flags: work
    docker compose up -d --wait {{ server }}
    GASHE_REDIS_ADDR=$(docker compose port {{ server }} 6379) go test -race -count=1 "${@:2}" ./...

# Run the Redis Cluster tests against three masters in Docker.
[group('redis')]
[working-directory('backend/redis')]
redis-cluster *flags: work
    docker compose up -d --wait cluster
    GASHE_REDIS_CLUSTER=127.0.0.1:17000,127.0.0.1:17001,127.0.0.1:17002 go test -race -count=1 -run Cluster "$@" ./...

# Run the Redis tests against Redis and Valkey at once.
[group('redis')]
[parallel]
redis-all: (redis-test "redis") (redis-test "valkey")

# Stop the Redis servers.
[group('redis')]
[working-directory('backend/redis')]
redis-down:
    docker compose down

# Lint the Markdown of docs/en/; the rules are in docs/pyproject.toml.
[group('docs')]
docs-lint:
    {{ rumdl }} check --config docs/pyproject.toml docs/en

# Fix what docs-lint finds and can fix on its own.
[group('docs')]
docs-fmt:
    {{ rumdl }} fmt --config docs/pyproject.toml docs/en

# Build the site into docs/site/; a broken link fails the build.
[group('docs')]
docs-build:
    {{ zensical }} build --strict -f docs/zensical.toml

# Preview the site at http://localhost:8000/gashe/.
[group('docs')]
docs-serve:
    {{ zensical }} serve -f docs/zensical.toml

# Write the API reference, docs/en/reference/, from the doc comments; run it after an API change.
[group('docs')]
docs-reference:
    for p in . backend/memory backend/nearcache httpcache backendtest codec/raw compress/gzip; do n=$(basename "$p"); [ "$n" = . ] && n=gashe; {{ gomarkdoc }} --output "docs/en/reference/$n.md" "./$p"; done
    for m in backend/redis codec/msgpack compress/zstd; do (cd "$m" && {{ gomarkdoc }} --output "$OLDPWD/docs/en/reference/$(basename "$m").md" .); done

# Fail when docs/en/reference/ does not match the doc comments.
[group('docs')]
docs-reference-check: docs-reference
    git diff --exit-code docs/en/reference

# Check the external links with lychee, in Docker.
[group('docs')]
docs-links:
    docker run --rm -v "$PWD":/repo -w /repo lycheeverse/lychee:0.24.2 --config docs/lychee.toml --no-progress README.md CHANGELOG.md CONTRIBUTING.md examples/README.md 'docs/*/**/*.md'
