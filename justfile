set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

modules := ". backend/redis codec/msgpack compress/zstd examples benchmarks"
released := ". backend/redis codec/msgpack compress/zstd"

# golang.org/x/exp has no tags, so gorelease is pinned to a commit.
govulncheck := "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
gorelease := "golang.org/x/exp/cmd/gorelease@v0.0.0-20261014143641-f0251eb4aa3e"
# CI checks out a pull request on no branch, so gomarkdoc cannot detect the repository.
gomarkdoc := "go run github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0 --repository.url https://github.com/eugeneliukindev/gashe --repository.default-branch main"

golangci := 'docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -e GOPATH=/tmp/go -e GOMODCACHE=/go/pkg/mod -v "$PWD":/src -w /src -v "$(go env GOMODCACHE)":/go/pkg/mod -v "$(go env GOCACHE)":/cache/go -e GOCACHE=/cache/go -v "$HOME/.cache/golangci-lint":/cache/lint -e GOLANGCI_LINT_CACHE=/cache/lint golangci/golangci-lint:v2.15.0'
actionlint := 'docker run --rm -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.12'
lychee := 'docker run --rm -e GITHUB_TOKEN -v "$PWD":/repo -w /repo lycheeverse/lychee:0.24.2'

zensical := "uv run --project docs --locked zensical"
rumdl := "uv run --project docs --locked rumdl"
tombi := "uv run --project docs --locked tombi"
typos := "uv run --project docs --locked typos"
prek := "uv run --project docs --locked prek"
commitizen := "uv run --project " + justfile_directory() + "/docs --locked cz"

needs_service := 'redis/go-redis|jackc/pgx|ListenAndServe|signal\.Notify'

# Lint Go code
[group('lint')]
golangci-lint: (_golangci "run")
    {{ golangci }} golangci-lint config verify

# Format Go code
[group('lint')]
golangci-fmt: (_golangci "fmt")

# Check typos
[group('lint')]
typos:
    {{ typos }}

# Check TOML format and schemas
[group('lint')]
tombi-lint:
    {{ tombi }} format --check
    {{ tombi }} lint

# Format TOML
[group('lint')]
tombi-fmt:
    {{ tombi }} format

# Check GitHub workflows
[group('lint')]
actionlint:
    {{ actionlint }}

# Check a commit message: the commit-msg hook
[group('lint')]
commitizen-check file:
    {{ commitizen }} check --commit-msg-file "$1"

# Run tests with -race, without Redis
[group('test')]
test: _work
    go test -race ./... ./backend/redis/... ./codec/msgpack/... ./compress/zstd/...

# Run examples that need no server
[group('test')]
[working-directory('examples')]
examples: _work
    for e in $(find . -name main.go -exec dirname {} \; | sort); do sed 's#//.*##' "$e"/*.go | grep -qE '{{ needs_service }}' && continue; echo "== example $e"; go run "$e" > /dev/null; done

# Run benchmarks: just bench -count=5
[group('test')]
[working-directory('benchmarks')]
bench *flags: _work
    go test -run '^$' -bench . -benchtime 300ms "$@" .

# Run Redis tests: just redis-test valkey -run Lock
[group('redis')]
[working-directory('backend/redis')]
redis-test server="redis" *flags: _work
    docker compose up -d --wait {{ server }}
    GASHE_REDIS_ADDR=$(docker compose port {{ server }} 6379) go test -race -count=1 "${@:2}" ./...

# Run Redis Cluster tests
[group('redis')]
[working-directory('backend/redis')]
redis-cluster *flags: _work
    docker compose up -d --wait cluster
    GASHE_REDIS_CLUSTER=127.0.0.1:17000,127.0.0.1:17001,127.0.0.1:17002 go test -race -count=1 -run Cluster "$@" ./...

# Run Redis tests on Redis and Valkey
[group('redis')]
[parallel]
redis-all: (redis-test "redis") (redis-test "valkey")

# Stop Redis servers
[group('redis')]
[working-directory('backend/redis')]
redis-down:
    docker compose down

# Check known vulnerabilities
[group('security')]
govulncheck: (_each released "env" "GOWORK=off" "go" "run" govulncheck "./...")

# Lint docs Markdown
[group('docs')]
rumdl-lint:
    {{ rumdl }} check --config docs/pyproject.toml docs/en

# Fix docs Markdown
[group('docs')]
rumdl-fmt:
    {{ rumdl }} fmt --config docs/pyproject.toml docs/en

# Generate API reference from doc comments
[group('docs')]
gomarkdoc: _work
    mkdir -p docs/en/reference
    for p in . backend/memory backend/nearcache httpcache backendtest codec/raw compress/gzip; do n=$(basename "$p"); [ "$n" = . ] && n=gashe; {{ gomarkdoc }} --repository.path / --output "docs/en/reference/$n.md" "./$p"; done
    for m in backend/redis codec/msgpack compress/zstd; do (cd "$m" && {{ gomarkdoc }} --repository.path "/$m/" --output "$OLDPWD/docs/en/reference/$(basename "$m").md" .); done

# Build docs site
[group('docs')]
zensical-build: gomarkdoc
    {{ zensical }} build --strict -f docs/zensical.toml

# Serve docs at http://localhost:8000/gashe/
[group('docs')]
zensical-serve: gomarkdoc
    {{ zensical }} serve -f docs/zensical.toml

# Check external links
[group('docs')]
lychee:
    {{ lychee }} --config docs/lychee.toml --no-progress README.md CHANGELOG.md CONTRIBUTING.md examples/README.md 'docs/*/**/*.md'

# Tidy go.mod of every module
[group('deps')]
tidy: (_each modules "env" "GOWORK=off" "go" "mod" "tidy")

# Compare the API with the last tags, on a clean tree
[group('release')]
gorelease: (_each released "env" "GOWORK=off" "go" "run" gorelease)

# Tag the next version of a module: just commitizen-bump backend/redis
[group('release')]
commitizen-bump module="." increment="USE_GIT_COMMITS":
    cd "$1" && t=$({{ commitizen }} version --project --tag --next "$2") && git tag -a "$t" -m "$t" && echo "$t"

# Lint and test without Docker servers, before a commit
[group('all')]
check-commit: golangci-lint typos tombi-lint actionlint rumdl-lint test govulncheck

# Run everything CI runs, before a release tag
[group('all')]
check-release: check-commit examples redis-all redis-cluster zensical-build gorelease

# Install the git hooks of prek.toml
[group('hooks')]
prek-install:
    {{ prek }} install

# Run the git hooks on all files
[group('hooks')]
prek-run:
    {{ prek }} run --all-files

[default]
_list:
    @just --list --unsorted

# Create go.work, so every module builds against the local code of the others.
_work:
    [ -f go.work ] || { go work init {{ modules }} && go work edit -go=1.25.0 -toolchain=none; }

_each dirs +cmd:
    for d in {{ dirs }}; do echo "== $d"; (cd "$d" && "${@:2}"); done

_golangci cmd: _work
    mkdir -p "$(go env GOMODCACHE)" "$(go env GOCACHE)" "$HOME/.cache/golangci-lint"
    {{ golangci }} sh -c 'for d in {{ modules }}; do echo "== $d"; (cd "$d" && golangci-lint {{ cmd }} ./...); done'
