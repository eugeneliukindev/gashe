set shell := ["bash", "-euo", "pipefail", "-c"]
set positional-arguments

modules := ". backend/redis codec/msgpack compress/zstd examples benchmarks"
released := ". backend/redis codec/msgpack compress/zstd"

govulncheck := "golang.org/x/vuln/cmd/govulncheck@v1.9.0"
# gomarkdoc cannot detect the repository on a detached HEAD.
gomarkdoc := "go run github.com/princjef/gomarkdoc/cmd/gomarkdoc@v1.1.0 --repository.url https://github.com/eugeneliukindev/gashe --repository.default-branch main"

golangci := 'docker run --rm --user "$(id -u):$(id -g)" -e HOME=/tmp -e GOPATH=/tmp/go -e GOMODCACHE=/go/pkg/mod -v "$PWD":/src -w /src -v "$(go env GOMODCACHE)":/go/pkg/mod -v "$(go env GOCACHE)":/cache/go -e GOCACHE=/cache/go -v "$HOME/.cache/golangci-lint":/cache/lint -e GOLANGCI_LINT_CACHE=/cache/lint golangci/golangci-lint:v2.15.0'
actionlint := 'docker run --rm -v "$PWD":/repo -w /repo rhysd/actionlint:1.7.12'
lychee := 'docker run --rm -e GITHUB_TOKEN -v "$PWD":/repo -w /repo lycheeverse/lychee:0.24.2'

zensical := "uv run --project docs --locked zensical"
rumdl := "uv run --project docs --locked rumdl"
tombi := "uv run --project docs --locked tombi"
typos := "uv run --project docs --locked typos"
prek := "uv run --project docs --locked prek"
commitizen := "uv run --project docs --locked cz"

needs_service := 'redis/go-redis|jackc/pgx|ListenAndServe|signal\.Notify'

[doc("Run every linter")]
[group('all')]
lint: golangci-lint typos tombi-check actionlint rumdl-check

[doc("Run every formatter")]
[group('all')]
fmt: golangci-fmt tombi-fmt rumdl-fmt

[doc("Lint and test without Docker servers, before a commit")]
[group('all')]
check: lint test govulncheck

[doc("Run every check, before a release")]
[group('all')]
check-all: check test-examples test-redis-all test-cluster zensical-build

[doc("Lint Go code")]
[group('lint')]
golangci-lint: (_golangci "run")
    {{ golangci }} golangci-lint config verify

[doc("Format Go code")]
[group('lint')]
golangci-fmt: (_golangci "fmt")

[doc("Check typos")]
[group('lint')]
typos:
    {{ typos }}

[doc("Check TOML format and schemas")]
[group('lint')]
tombi-check:
    {{ tombi }} format --check
    {{ tombi }} lint

[doc("Format TOML")]
[group('lint')]
tombi-fmt:
    {{ tombi }} format

[doc("Lint docs Markdown")]
[group('lint')]
rumdl-check:
    {{ rumdl }} check --config docs/pyproject.toml docs/en

[doc("Fix docs Markdown")]
[group('lint')]
rumdl-fmt:
    {{ rumdl }} fmt --config docs/pyproject.toml docs/en

[doc("Check GitHub workflows")]
[group('lint')]
actionlint:
    {{ actionlint }}

[doc("Check a commit message: the commit-msg hook")]
[group('lint')]
commitizen-check file:
    {{ commitizen }} check --commit-msg-file "$1"

[doc("Run tests with -race, without Redis")]
[group('test')]
test: _work
    go test -race ./... ./backend/redis/... ./codec/msgpack/... ./compress/zstd/...

[doc("Run examples that need no server")]
[group('test')]
[working-directory('examples')]
test-examples: _work
    for e in $(find . -name main.go -exec dirname {} \; | sort); do sed 's#//.*##' "$e"/*.go | grep -qE '{{ needs_service }}' && continue; echo "== example $e"; go run "$e" > /dev/null; done

[doc("Run Redis tests in Docker: just test-redis valkey -run Lock")]
[group('test')]
[working-directory('backend/redis')]
test-redis server="redis" *flags: _work
    docker compose up -d --wait {{ server }}
    GASHE_REDIS_ADDR=$(docker compose port {{ server }} 6379) go test -race -count=1 "${@:2}" ./...

[doc("Run Redis tests on Redis and Valkey")]
[group('test')]
[parallel]
test-redis-all: (test-redis "redis") (test-redis "valkey")

[doc("Run Redis Cluster tests in Docker")]
[group('test')]
[working-directory('backend/redis')]
test-cluster *flags: _work
    docker compose up -d --wait cluster
    GASHE_REDIS_CLUSTER=127.0.0.1:17000,127.0.0.1:17001,127.0.0.1:17002 go test -race -count=1 -run Cluster "$@" ./...

[doc("Stop the Redis servers")]
[group('test')]
[working-directory('backend/redis')]
redis-down:
    docker compose down

[doc("Run benchmarks: just bench -count=5")]
[group('test')]
[working-directory('benchmarks')]
bench *flags: _work
    go test -run '^$' -bench . -benchtime 300ms "$@" .

[doc("Check known vulnerabilities")]
[group('security')]
govulncheck: (_each released "env" "GOWORK=off" "go" "run" govulncheck "./...")

[doc("Generate the API reference from doc comments")]
[group('docs')]
gomarkdoc: _work
    mkdir -p docs/en/reference
    for p in . backend/memory backend/nearcache httpcache backendtest codec/raw compress/gzip; do n=$(basename "$p"); [ "$n" = . ] && n=gashe; {{ gomarkdoc }} --repository.path / --output "docs/en/reference/$n.md" "./$p"; done
    for m in backend/redis codec/msgpack compress/zstd; do (cd "$m" && {{ gomarkdoc }} --repository.path "/$m/" --output "$OLDPWD/docs/en/reference/$(basename "$m").md" .); done

[doc("Build the docs site")]
[group('docs')]
zensical-build: gomarkdoc
    {{ zensical }} build --strict -f docs/zensical.toml

[doc("Serve the docs at http://localhost:8000/gashe/")]
[group('docs')]
zensical-serve: gomarkdoc
    {{ zensical }} serve -f docs/zensical.toml

[doc("Check external links")]
[group('docs')]
lychee:
    {{ lychee }} --config docs/lychee.toml --no-progress README.md CHANGELOG.md CONTRIBUTING.md examples/README.md 'docs/*/**/*.md'

[doc("Tidy go.mod of every module")]
[group('deps')]
tidy: (_each modules "env" "GOWORK=off" "go" "mod" "tidy")

[doc("Ask the Go proxy for a released tag, so pkg.go.dev lists it: just version-fetch v0.7.0")]
[group('release')]
version-fetch tag:
    m=github.com/eugeneliukindev/gashe; [[ "$1" == v* ]] || m="$m/${1%/v*}"; cd "$(mktemp -d)" && GOPROXY=https://proxy.golang.org go list -m "$m@${1##*/}"

[doc("Draft a GitHub release on main; publishing it creates the tag: just gh-release v0.7.0")]
[group('release')]
gh-release tag:
    #!/usr/bin/env bash
    set -euo pipefail
    git fetch -q --tags origin
    if [[ "$1" == v* ]]; then
        notes=$(awk -v h="## [${1#v}]" 'index($0, h) == 1 { f = 1; next } /^## \[/ { f = 0 } f' CHANGELOG.md)
        [ -n "$notes" ] || { echo "gh-release: CHANGELOG.md has no section ${1#v}" >&2; exit 1; }
        printf '%s\n' "$notes" | gh release create "$1" --draft --target main --title "$1" --notes-file -
    else
        last=$(git tag -l "${1%/v*}/v*" --sort=-v:refname | head -1)
        gh release create "$1" --draft --target main --title "$1" --generate-notes ${last:+--notes-start-tag "$last"}
    fi

[doc("Install the git hooks of prek.toml")]
[group('hooks')]
prek-install:
    {{ prek }} install

[doc("Run the git hooks on all files")]
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
