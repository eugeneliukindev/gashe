# Development tasks for gashe. Run `just` to list them, `just check` before a commit and
# `just release-check` before a tag.

set shell := ["bash", "-euo", "pipefail", "-c"]

# Every Go module of the repository.
modules := ". backend/redis backend/redis/integration codec/msgpack"

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
lint: lint-config
    for m in {{ modules }}; do echo "== lint $m"; (cd "$m" && golangci-lint run ./...); done

# Run the tests of every module except the integration one, with the race detector.
[group('test')]
test:
    for m in {{ released }}; do echo "== test $m"; (cd "$m" && go test -race ./...); done

# Run the tests on the oldest supported Go; needs goenv.
[group('test')]
test-oldest:
    for m in {{ released }}; do echo "== test $m on Go {{ oldest_go }}"; (cd "$m" && GOENV_VERSION={{ oldest_go }} GOTOOLCHAIN=local go test -race ./...); done

# Run the Redis integration tests in Docker; pass another image to test a compatible server.
[group('test')]
integration image="redis:7.0.0":
    cd backend/redis/integration && GASHE_REDIS_IMAGE={{ image }} go test -race ./...

# Report known vulnerabilities in the code and dependencies the modules call.
[group('security')]
vuln:
    for m in {{ released }}; do echo "== govulncheck $m"; (cd "$m" && {{ govulncheck }} ./...); done

# Tidy go.mod and go.sum of every module.
[group('deps')]
tidy:
    for m in {{ modules }}; do (cd "$m" && go mod tidy); done

# Compare the API of released modules with their last tags; needs a clean working tree.
[group('release')]
api:
    for m in {{ released }}; do echo "== gorelease $m"; (cd "$m" && {{ gorelease }}); done

# Everything to run before a commit.
[group('all')]
check: lint test vuln

# Everything to run before a release tag.
[group('all')]
release-check: check test-oldest integration api
