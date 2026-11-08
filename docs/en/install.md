# Install { #install }

gashe needs Go 1.25 or newer. The modules are separate, so each brings only the dependency of
what you use:

--8<-- "docs/snippets/install.md"

| Module | What it adds | Dependency |
| --- | --- | --- |
| `gashe` | the cache, the memory backend, the near cache, the HTTP middleware, JSON, gzip | none |
| `gashe/backend/redis` | Redis and Valkey, a Cluster included | go-redis v9 |
| `gashe/codec/msgpack` | values in MessagePack | vmihailenco/msgpack |
| `gashe/compress/zstd` | values compressed with Zstandard | klauspost/compress |

Features marked <span class="version-badge">Added in v0.5</span> need that version or newer; the
[release notes](changelog.md) list what each version changed.
