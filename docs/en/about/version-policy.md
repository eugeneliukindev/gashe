# Version policy { #version-policy }

How gashe is versioned, which Go and Redis versions it supports, and what counts as breaking.

## Versions { #versions }

Versions follow [Semantic Versioning](https://semver.org/). While the version is `0.x`, a minor
release may change the API: `v0.3.0` to `v0.4.0` can break code. No `v1.0.0` date is set.

| Change | Where it is said |
|---|---|
| Every release | the [changelog](../changelog.md), copied into the GitHub release of its tag |
| Code to change on upgrade | the changelog, and [Migration](migration.md) as before -> after tables |
| Versions not to use | `retract` in `go.mod`: `go get` skips them, `go list -m -u` points past them |

## Modules { #modules }

Four modules are released, each with its own tags. A module gets a new tag only when it changes, so
their versions differ.

| Module | Import path | Tag | Holds |
|---|---|---|---|
| root | `github.com/eugeneliukindev/gashe` | `v0.4.0` | `gashe`, `backend/memory`, `backend/nearcache`, `backendtest`, `httpcache`, `codec/raw`, `compress/gzip` |
| Redis | `github.com/eugeneliukindev/gashe/backend/redis` | `backend/redis/v0.3.0` | the Redis backend and buses, on go-redis v9 |
| MessagePack | `github.com/eugeneliukindev/gashe/codec/msgpack` | `codec/msgpack/v0.3.0` | a MessagePack codec |
| Zstandard | `github.com/eugeneliukindev/gashe/compress/zstd` | `compress/zstd/v0.1.0` | a Zstandard compressor |

- **Order:** the root is tagged first; the other modules then require that released version of the
  root and get their own tags.
- **Together:** a module may require a newer root than you have: `compress/zstd` requires gashe
  `v0.4.0`; `go get` raises the root for you.
- **Retracted:** `v0.1.0` and `v0.2.0` of the root, `backend/redis/v0.1.0` and
  `backend/redis/v0.2.0`, `codec/msgpack/v0.1.0`: their API broke before `v0.3.0`.
- **Dependencies:** the root module has none; the other three each need one library, see their
  `go.mod`.

## Go versions { #go-versions }

| What | Go |
|---|---|
| Lowest | Go 1.23, the `go` line of every `go.mod` |
| Supported | Go 1.23 and newer: five releases at the moment |
| Tested | before every tag, on Go 1.23 and on the Go that makes the release |
| Not decided | when the lowest version is raised |

## Redis and Valkey servers { #redis-and-valkey-servers }

`backend/redis` requires go-redis `v9.0.0` up to `backend/redis/v0.3.0`, and `v9.18.0` from its next
release. Which servers are tested, and what a server needs:
[`backend/redis`](../concepts/backends.md#backendredis).

## What counts as a breaking change { #what-counts-as-a-breaking-change }

The changes that broke an upgrade so far:

| Kind | Example from the changelog |
|---|---|
| A name removed or renamed | 0.3.0: `Incr` becomes `IncrBy`; 0.4.0: `Gzip` is removed |
| A signature or field type changed | 0.2.0: `Options.Hooks.OnResult` receives an `Outcome`; 0.3.0: `Policy.TTL.For` takes several types |
| A method of an interface you implement renamed | 0.3.0: a backend of yours renames `Incr` to `IncrBy` |
| Keys or entries stored differently | 0.2.0 and 0.3.0: rate limit counters start from zero; 0.3.0: tagged entries reload once |
| A higher required version of a dependency | unreleased: go-redis `v9.18.0` instead of `v9.0.0` |
| A behaviour changed | unreleased: `httpcache` no longer caches requests with a `Cookie` header |

Stored data counts: an upgrade can make old entries misses, so they load again, or reset counters.
The changelog says what each release does to stored data. Whether `0.x` releases keep the stored
format is not decided; read the changelog before you upgrade processes that share one Redis.

Not breaking, as the changelog of 0.2.0 says: results named in signatures, since calls do not
change.
