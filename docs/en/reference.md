# API reference { #api-reference }

The reference of every type and function is on pkg.go.dev, built from the doc comments:

| Package | What it is |
|---|---|
| [`gashe`](https://pkg.go.dev/github.com/eugeneliukindev/gashe) | the cache, `GetOrLoad`, `Policy`, locks, limits, breakers |
| [`gashe/backend/memory`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/backend/memory) | the in-process backend |
| [`gashe/backend/redis`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/backend/redis) | the Redis backend and its buses |
| [`gashe/backend/nearcache`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/backend/nearcache) | a memory cache in front of a shared backend |
| [`gashe/backendtest`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/backendtest) | the contract suite for your own backend |
| [`gashe/httpcache`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/httpcache) | the HTTP middleware |
| [`gashe/codec/msgpack`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/codec/msgpack) | the MessagePack codec |
| [`gashe/codec/raw`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/codec/raw) | stores `[]byte` and `string` values as they are |
| [`gashe/compress/gzip`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/compress/gzip) | the gzip compressor |
| [`gashe/compress/zstd`](https://pkg.go.dev/github.com/eugeneliukindev/gashe/compress/zstd) | the Zstandard compressor |
