# Development tasks for gashe. Run `just` to list them, `just check` before a commit and
# `just release-check` before a tag.

set shell := ["bash", "-euo", "pipefail", "-c"]
set script-interpreter := ["bash", "-euo", "pipefail"]

# Every Go module of the repository.
modules := ". backend/redis codec/msgpack compress/zstd examples benchmarks"

# The modules users import, released with their own tags.
released := ". backend/redis codec/msgpack compress/zstd"

# The oldest Go the modules support, run through goenv.
oldest_go := "1.23.12"

# Tools run with go run at pinned versions, so every run checks the same way. golang.org/x/exp
# has no tags, so gorelease is pinned to a commit.
govulncheck := "go run golang.org/x/vuln/cmd/govulncheck@v1.9.0"
gorelease := "go run golang.org/x/exp/cmd/gorelease@v0.0.0-20261014143641-f0251eb4aa3e"

# The documentation site is built by Zensical, run through uv at a pinned version.
zensical := "uv run --no-project --with zensical==0.1.0 python docs/build.py"

[default]
[private]
list:
    @just --list

# Create go.work, so every module builds against the local code of the others; it is not committed.
[private]
work:
    [ -f go.work ] || { go work init {{ modules }} && go work edit -go=1.23 -toolchain=none; }

# Format the code with the formatters of .golangci.yml.
[group('lint')]
fmt:
    for m in {{ modules }}; do (cd "$m" && golangci-lint fmt ./...); done

# Check that .golangci.yml is valid.
[group('lint')]
lint-config:
    golangci-lint config verify

# Run golangci-lint in every module.
[group('lint')]
lint: work lint-config
    for m in {{ modules }}; do echo "== lint $m"; (cd "$m" && golangci-lint run ./...); done

# Run the tests of every released module with the race detector; tests that need Redis are skipped.
[group('test')]
test: work
    for m in {{ released }}; do echo "== test $m"; (cd "$m" && go test -race ./...); done

# Run the tests on the oldest supported Go; needs goenv.
[env('GOENV_VERSION', oldest_go)]
[env('GOTOOLCHAIN', 'local')]
[group('test')]
test-oldest: work
    for m in {{ released }}; do echo "== test $m on Go {{ oldest_go }}"; (cd "$m" && go test -race ./...); done

# Run the backend/redis tests against a server in Docker, on the local gashe code; flags go to go test.
[group('test')]
[positional-arguments]
[script]
[working-directory('backend/redis')]
integration image="redis:7.0.0" *flags: work
    id=$(docker run -d --rm -p 127.0.0.1::6379 {{ image }})
    trap 'docker rm -f "$id" > /dev/null' EXIT
    port=$(docker port "$id" 6379/tcp | head -1 | sed 's/.*://')
    for _ in $(seq 100); do # wait until the server answers PING
        if exec 3<> "/dev/tcp/127.0.0.1/$port" 2> /dev/null && printf 'PING\r\n' >&3 && read -r -t 1 reply <&3 && [[ $reply == +PONG* ]]; then
            break
        fi
        sleep 0.1
    done
    echo "== {{ image }} on 127.0.0.1:$port"
    GASHE_REDIS_ADDR="127.0.0.1:$port" go test -race -count=1 "${@:2}" ./...

# Run the backend/redis tests against a Redis Cluster of three masters in Docker; flags go to go test.
# The nodes listen on 127.0.0.1:17000-17002 of the host network, so the addresses the cluster
# announces are the ones the tests reach.
[group('test')]
[positional-arguments]
[script]
[working-directory('backend/redis')]
integration-cluster image="redis:7.0.0" *flags: work
    id=$(docker run -d --rm --network host {{ image }} sh -c '
        for p in 17000 17001 17002; do
            redis-server --port $p --bind 127.0.0.1 --cluster-enabled yes --cluster-config-file /tmp/nodes-$p.conf --save "" --daemonize yes
        done
        sleep 1
        redis-cli --cluster create 127.0.0.1:17000 127.0.0.1:17001 127.0.0.1:17002 --cluster-replicas 0 --cluster-yes > /dev/null
        exec sleep infinity')
    trap 'docker rm -f "$id" > /dev/null' EXIT
    for p in 17000 17001 17002; do # wait until every node serves every slot
        for _ in $(seq 100); do
            docker exec "$id" redis-cli -p "$p" cluster info 2> /dev/null | grep -q 'cluster_state:ok' && break
            sleep 0.2
        done
    done
    echo "== {{ image }} cluster on 127.0.0.1:17000-17002"
    GASHE_REDIS_CLUSTER="127.0.0.1:17000,127.0.0.1:17001,127.0.0.1:17002" go test -race -count=1 -run Cluster "${@:2}" ./...

# Run the integration tests on the oldest supported Redis and on Valkey, at once.
[group('test')]
[parallel]
integration-all: (integration "redis:7.0.0") (integration "valkey/valkey:8")

# Run every example that needs no Redis, so they keep working with the current API.
[group('test')]
[working-directory('examples')]
examples: work
    for e in */ tutorial/*/; do case "$e" in nearcache/|httpapi/|tutorial/) continue;; esac; echo "== example ${e%/}"; go run "./$e" > /dev/null; done

# Run the benchmarks; flags go to go test, such as `just bench -count=5`. docs/en/internals/benchmarks.md has the results.
[group('test')]
[positional-arguments]
[working-directory('benchmarks')]
bench *flags: work
    go test -run '^$' -bench . -benchtime 300ms "$@" .

# Build the documentation site in every language into site/; a broken link fails the build.
[group('docs')]
docs:
    {{ zensical }} build

# Preview the documentation of one language at http://localhost:8000/gashe/.
[group('docs')]
docs-serve lang="en":
    {{ zensical }} serve {{ lang }}

# Run the examples and curl against examples/httpapi, and write their outputs into docs/snippets/.
[group('docs')]
docs-output: work
    python3 docs/outputs.py

# Check the external links of the documentation with lychee, in Docker.
[group('docs')]
docs-links:
    docker run --rm -v "$PWD":/repo -w /repo lycheeverse/lychee:0.24.2 --config lychee.toml --no-progress README.md CHANGELOG.md CONTRIBUTING.md examples/README.md 'docs/*/**/*.md'

# List translated pages whose English page changed since they were translated.
[group('docs')]
docs-check:
    {{ zensical }} check

# Report known vulnerabilities in the code and dependencies the released modules call.
[env('GOWORK', 'off')]
[group('security')]
vuln:
    for m in {{ released }}; do echo "== govulncheck $m"; (cd "$m" && {{ govulncheck }} ./...); done

# Tidy go.mod and go.sum of every module.
[env('GOWORK', 'off')]
[group('deps')]
tidy:
    for m in {{ modules }}; do (cd "$m" && go mod tidy); done

# Compare the API of released modules with their last tags; needs a clean working tree.
[env('GOWORK', 'off')]
[group('release')]
api:
    for m in {{ released }}; do echo "== gorelease $m"; (cd "$m" && {{ gorelease }}); done

# Everything to run before a commit.
[group('all')]
check: lint test vuln

# Everything to run before a release tag.
[group('all')]
release-check: check examples test-oldest integration-all integration-cluster api
