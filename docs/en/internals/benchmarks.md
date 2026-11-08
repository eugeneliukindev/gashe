# Benchmarks { #benchmarks }

How fast and how small every codec and compressor is, to choose [`Codec`](../reference/gashe.md#Codec) and [`Compressor`](../reference/gashe.md#Compressor).

| What | How |
|---|---|
| Runs | the median of 5 runs of `go test -run '^$' -bench . -benchtime 300ms -count=5` in `benchmarks/` on an Intel Core i9-9900KF, Go 1.26, linux/amd64 |
| Code | [benchmarks](https://github.com/eugeneliukindev/gashe/tree/main/benchmarks/) |
| Data | generated from 12 words, so it compresses better than real text does |

**A user**, 4 fields:

| Codec + compressor | Size | Encode | Decode |
|---|---|---|---|
| json | 68 B | 238 ns | 1.1 us |
| json + gzip | 92 B | 16.4 us | 2.4 us |
| json + zstd | 81 B | 1.1 us | 1.2 us |
| msgpack | 49 B | 262 ns | 422 ns |
| msgpack + gzip | 74 B | 16.2 us | 1.2 us |
| msgpack + zstd | 62 B | 1.1 us | 610 ns |

**A product** with a description of 150 words:

| Codec + compressor | Size | Encode | Decode |
|---|---|---|---|
| json | 972 B | 1.3 us | 5.5 us |
| json + gzip | 346 B | 30.4 us | 12.1 us |
| json + zstd | 341 B | 10.2 us | 9.3 us |
| msgpack | 945 B | 512 ns | 732 ns |
| msgpack + gzip | 339 B | 30.7 us | 7.2 us |
| msgpack + zstd | 331 B | 10.2 us | 4.9 us |

**A list** of 500 products:

| Codec + compressor | Size | Encode | Decode |
|---|---|---|---|
| json | 108.2 KB | 204 us | 1.01 ms |
| json + gzip | 20.1 KB | 4.00 ms | 1.31 ms |
| json + zstd | 22.3 KB | 800 us | 1.16 ms |
| msgpack | 93.7 KB | 163 us | 285 us |
| msgpack + gzip | 19.5 KB | 4.49 ms | 573 us |
| msgpack + zstd | 22.6 KB | 712 us | 395 us |

**Recommended:**

| Choice | Pick | Why |
|---|---|---|
| Codec | `msgpack` | it decodes 2 to 7 times faster than JSON at every size, and every hit decodes; keep JSON where people or other services read the entries |
| Compressor | none for small values, `zstd` for large ones | a value of a few dozen bytes, such as the user, gets bigger and slower; from about 1 KB zstd stores 3 to 5 times less here, for under a millisecond even on the list |
| gzip | only where a third-party dependency is not allowed | it stores about the same as zstd but encodes 3 to 15 times slower |
| Small and large values | two caches, such as one with `Compressor: zstd.New()` for pages and lists | one [`Cache`](../reference/gashe.md#Cache) compresses all its values |
