# API reference { #api-reference }

Every type, field and function, from the doc comments of the code: `just docs-reference` writes
these pages with [gomarkdoc](https://github.com/princjef/gomarkdoc). The same is on
[pkg.go.dev](https://pkg.go.dev/github.com/eugeneliukindev/gashe).

| Package | What it is |
|---|---|
| [`gashe`](reference/gashe.md) | the cache, `GetOrLoad`, `Policy`, locks, limits, breakers |
| [`gashe/backend/memory`](reference/memory.md) | the in-process backend |
| [`gashe/backend/redis`](reference/redis.md) | the Redis backend and its buses |
| [`gashe/backend/nearcache`](reference/nearcache.md) | a memory cache in front of a shared backend |
| [`gashe/httpcache`](reference/httpcache.md) | the HTTP middleware |
| [`gashe/backendtest`](reference/backendtest.md) | the contract suite for your own backend |
| [`gashe/codec/msgpack`](reference/msgpack.md) | the MessagePack codec |
| [`gashe/codec/raw`](reference/raw.md) | stores `[]byte` and `string` values as they are |
| [`gashe/compress/gzip`](reference/gzip.md) | the gzip compressor |
| [`gashe/compress/zstd`](reference/zstd.md) | the Zstandard compressor |
