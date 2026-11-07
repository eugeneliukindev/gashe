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
commitizen := "uv run --project " + justfile_directory() + "/docs --locked cz"

needs_service := 'redis/go-redis|jackc/pgx|ListenAndServe|signal\.Notify'

[default]
[private]
list:
    @just --list

[private]
work:
    [ -f go.work ] || { go work init {{ modules }} && go work edit -go=1.25.0 -toolchain=none; }

[private]
each dirs +cmd:
    for d in {{ dirs }}; do echo "== $d"; (cd "$d" && "${@:2}"); done

[private]
golangci cmd: work
    mkdir -p "$(go env GOMODCACHE)" "$(go env GOCACHE)" "$HOME/.cache/golangci-lint"
    {{ golangci }} sh -c 'for d in {{ modules }}; do echo "== $d"; (cd "$d" && golangci-lint {{ cmd }} ./...); done'

# Format Go code
[group('lint')]
golangci-fmt: (golangci "fmt")

# Lint Go code
[group('lint')]
golangci-lint: (golangci "run")
    {{ golangci }} golangci-lint config verify

# Check TOML format and schemas
[group('lint')]
tombi-lint:
    {{ tombi }} format --check
    {{ tombi }} lint

# Format TOML
[group('lint')]
tombi-fmt:
    {{ tombi }} format

# Install git hooks
[group('lint')]
hooks:
    uv run --project docs --locked prek install

# Run git hooks on all files
[group('lint')]
hooks-all:
    uv run --project docs --locked prek run --all-files

# Check typos
[group('lint')]
typos:
    uv run --project docs --locked typos

# Check GitHub workflows
[group('lint')]
actionlint:
    {{ actionlint }}

# Run tests with -race, without Redis
[group('test')]
test: work
    go test -race ./... ./backend/redis/... ./codec/msgpack/... ./compress/zstd/...

# Run examples that need no server
[group('test')]
[working-directory('examples')]
examples: work
    for e in $(find . -name main.go -exec dirname {} \; | sort); do sed 's#//.*##' "$e"/*.go | grep -qE '{{ needs_service }}' && continue; echo "== example $e"; go run "$e" > /dev/null; done

# Run benchmarks: just bench -count=5
[group('test')]
[working-directory('benchmarks')]
bench *flags: work
    go test -run '^$' -bench . -benchtime 300ms "$@" .

# Check known vulnerabilities
[group('security')]
govulncheck: (each released "env" "GOWORK=off" "go" "run" govulncheck "./...")

# Tidy go.mod of every module
[group('deps')]
tidy: (each modules "env" "GOWORK=off" "go" "mod" "tidy")

# Compare the API with the last tags, on a clean tree
[group('release')]
gorelease: (each released "env" "GOWORK=off" "go" "run" gorelease)

# Run before a commit
[group('all')]
check: golangci-lint typos actionlint tombi-lint rumdl-lint test govulncheck

# Run before a release tag
[group('all')]
release-check: check examples redis-all redis-cluster gorelease

# Run Redis tests: just redis-test valkey -run Lock
[group('redis')]
[working-directory('backend/redis')]
redis-test server="redis" *flags: work
    docker compose up -d --wait {{ server }}
    GASHE_REDIS_ADDR=$(docker compose port {{ server }} 6379) go test -race -count=1 "${@:2}" ./...

# Run Redis Cluster tests
[group('redis')]
[working-directory('backend/redis')]
redis-cluster *flags: work
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

# Lint docs Markdown
[group('docs')]
rumdl-lint:
    {{ rumdl }} check --config docs/pyproject.toml docs/en

# Fix docs Markdown
[group('docs')]
rumdl-fmt:
    {{ rumdl }} fmt --config docs/pyproject.toml docs/en

# Build docs site
[group('docs')]
zensical-build: gomarkdoc
    {{ zensical }} build --strict -f docs/zensical.toml

# Serve docs at http://localhost:8000/gashe/
[group('docs')]
zensical-serve: gomarkdoc
    {{ zensical }} serve -f docs/zensical.toml

# Generate API reference from doc comments
[group('docs')]
gomarkdoc: work
    mkdir -p docs/en/reference
    for p in . backend/memory backend/nearcache httpcache backendtest codec/raw compress/gzip; do n=$(basename "$p"); [ "$n" = . ] && n=gashe; {{ gomarkdoc }} --repository.path / --output "docs/en/reference/$n.md" "./$p"; done
    for m in backend/redis codec/msgpack compress/zstd; do (cd "$m" && {{ gomarkdoc }} --repository.path "/$m/" --output "$OLDPWD/docs/en/reference/$(basename "$m").md" .); done

# Check external links
[group('docs')]
lychee:
    {{ lychee }} --config docs/lychee.toml --no-progress README.md CHANGELOG.md CONTRIBUTING.md examples/README.md 'docs/*/**/*.md'

# Check a commit message: the commit-msg hook
[group('lint')]
commitizen-check file:
    {{ commitizen }} check --commit-msg-file "$1"

# Tag the next version of a module, from its cz.toml and the commits since its last tag: just commitizen-bump backend/redis
[group('deploy')]
commitizen-bump module="." increment="USE_GIT_COMMITS":
    cd "$1" && t=$({{ commitizen }} version --project --tag --next "$2") && git tag -a "$t" -m "$t" && echo "$t"

# Tag the modules on origin/main and push the tags: just release . backend/redis
[group('deploy')]
release +modules: release-check
    git fetch -q origin main && test "$(git rev-parse HEAD)" = "$(git rev-parse origin/main)"
    for m in "$@"; do {{ just_executable() }} commitizen-bump "$m"; done
    git push --atomic origin $(git tag --points-at HEAD)
