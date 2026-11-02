# Development tasks for gashe. Run `just` to list them, `just check` before a commit and
# `just release-check` before a tag.

set shell := ["bash", "-euo", "pipefail", "-c"]

# Every Go module of the repository.
modules := ". backend/redis codec/msgpack examples"

# The modules users import, released with their own tags.
released := ". backend/redis codec/msgpack"

# The oldest Go the modules support, run through goenv.
oldest_go := "1.23.12"

# Tools run with go run at pinned versions, so every run checks the same way. golang.org/x/exp
# has no tags, so gorelease is pinned to a commit.
govulncheck := "go run golang.org/x/vuln/cmd/govulncheck@v1.9.0"
gorelease := "go run golang.org/x/exp/cmd/gorelease@v0.0.0-20261014143641-f0251eb4aa3e"

[private]
default:
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
[group('test')]
test-oldest: work
    for m in {{ released }}; do echo "== test $m on Go {{ oldest_go }}"; (cd "$m" && GOENV_VERSION={{ oldest_go }} GOTOOLCHAIN=local go test -race ./...); done

# The Redis-compatible servers the backend is tested against: the oldest Redis it supports and Valkey.
servers := "redis:7.0.0 valkey/valkey:8"

# Run the backend/redis tests against a server in Docker, on the local gashe code; flags go to go test.
[group('test')]
integration image="redis:7.0.0" flags="": work
    #!/usr/bin/env bash
    set -euo pipefail
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
    cd backend/redis && GASHE_REDIS_ADDR="127.0.0.1:$port" go test -race -count=1 {{ flags }} ./...

# Run the integration tests against every server in `servers`.
[group('test')]
integration-all:
    for s in {{ servers }}; do just integration "$s"; done

# Run every example that needs no Redis, so they keep working with the current API.
[group('test')]
examples: work
    for e in examples/*/; do e=$(basename "$e"); [ "$e" = nearcache ] && continue; echo "== example $e"; (cd examples && go run "./$e" > /dev/null); done

# Report known vulnerabilities in the code and dependencies the released modules call.
[group('security')]
vuln:
    for m in {{ released }}; do echo "== govulncheck $m"; (cd "$m" && GOWORK=off {{ govulncheck }} ./...); done

# Tidy go.mod and go.sum of every module.
[group('deps')]
tidy:
    for m in {{ modules }}; do (cd "$m" && GOWORK=off go mod tidy); done

# Compare the API of released modules with their last tags; needs a clean working tree.
[group('release')]
api:
    for m in {{ released }}; do echo "== gorelease $m"; (cd "$m" && GOWORK=off {{ gorelease }}); done

# Everything to run before a commit.
[group('all')]
check: lint test vuln

# Everything to run before a release tag.
[group('all')]
release-check: check examples test-oldest integration-all api
