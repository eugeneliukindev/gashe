---
hide:
  - navigation
  - toc
---

# gashe { #gashe }

<p align="center">
  <img src="assets/logo.svg" width="260" alt="The gashe gopher, its cheeks full of cached data">
</p>

<p align="center" markdown>
[![Go Reference](https://pkg.go.dev/badge/github.com/eugeneliukindev/gashe.svg)](https://pkg.go.dev/github.com/eugeneliukindev/gashe)
[![Release](https://img.shields.io/github/v/tag/eugeneliukindev/gashe?filter=v*&label=release)](https://github.com/eugeneliukindev/gashe/tags)
[![Go version](https://img.shields.io/github/go-mod/go-version/eugeneliukindev/gashe)](https://github.com/eugeneliukindev/gashe/blob/main/go.mod)
[![License](https://img.shields.io/github/license/eugeneliukindev/gashe)](https://github.com/eugeneliukindev/gashe/blob/main/LICENSE)
</p>

Caching for Go services that run as many processes. One [`*gashe.Cache`](reference/gashe.md#Cache) stores the results of
expensive calls, keeps answering while a dependency is slow or down, and guards dependencies with
locks, rate limits and circuit breakers. Each process has its own memory; over Redis, entries,
tags, locks and limits are shared by every process.

## Why gashe { #why-gashe }

<div class="grid cards" markdown>

-   :lucide-zap: **Fast**

    ---

    Hits come from Redis, or from the memory of the process with a near cache: a hot key costs no
    round trip.

-   :lucide-shield-check: **Protects your database**

    ---

    A thousand requests for a cold key make one query, across every process. Past the TTL, the old
    result answers at once while one process reloads it.

-   :lucide-life-buoy: **Keeps answering**

    ---

    The database is slow or down: the last result answers. Redis is down: the memory of the
    process does.

-   :lucide-refresh-cw: **Invalidation that works**

    ---

    One write drops every entry of a tag, and every process sees it.

-   :lucide-lock: **Guards in the same cache**

    ---

    Distributed locks, rate limits and circuit breakers, shared by every process: no second system
    to run.

-   :lucide-package: **No dependencies**

    ---

    The core needs only the Go standard library, from Go 1.25. One call, [`GetOrLoad`](reference/gashe.md#GetOrLoad), does the
    caching.

</div>

How gashe compares with other Go caches: [Alternatives](about/alternatives.md).

## Install { #install }

--8<-- "docs/snippets/install.md"

Only the core is needed; each other module is added for what you use, with its own dependency.

Features marked <span class="version-badge">Added in v0.5</span> need v0.5.0 or newer. See the
[changelog](changelog.md).

## Example { #example }

A user is loaded once, then answered from Redis by every process for an hour; after that, the
stale result answers at once while one process reloads it.

```go
--8<-- "examples/docs/index/example/main.go:example"
```

1. For 10 minutes after the TTL, the stale result answers at once while one process reloads it in the background.
2. A cold key loads once for all processes.

??? example "Full program"

    ```go
    --8<-- "examples/docs/index/example/main.go"
    ```

## What it solves { #what-it-solves }

**Basics**

- The same expensive call again and again: [Load and cache data](concepts/loading.md#getorload)
- Each kind of data needs its own rules, set once: [The policy](concepts/loading.md#policy)
- N+1 queries on a list page: [Load and cache data](concepts/loading.md#getorloadmany)
- One change makes many entries stale: [Update and remove data](concepts/invalidation.md#tags)
- A hot key costs a round trip on every read: [Backends](concepts/backends.md#backendnearcache)
- A deploy changed a cached type: [Load and cache data](concepts/loading.md#type-shapes)

**Resilience**

- Every request waits when a popular key expires: [Stale data](concepts/stale.md#stalewhilerevalidate)
- The database is slow or down: [Stale data](concepts/stale.md#staleiferror-staleifslow)
- A cold key sends a thousand identical queries (cache stampede): [Many loads at once](concepts/stampedes.md)
- Keys stored together expire together (thundering herd): [Many loads at once](concepts/stampedes.md#jitter)
- Lookups of missing ids always reach the database (cache penetration): [Cache "not found"](concepts/negative.md)
- The cache backend is down: [When Redis is down](concepts/outages.md)

**Protection**

- A job must run once at a time across processes: [Locks](concepts/locks.md)
- A client calls far too often: [Rate limits](concepts/rate-limits.md)
- A failing dependency drags the service down (cascading failure): [Protect a dependency](concepts/breakers.md)

**HTTP**

- A heavy page is rendered on every request: [HTTP caching](concepts/httpcache.md)
