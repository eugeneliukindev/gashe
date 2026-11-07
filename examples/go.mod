// Runnable examples of gashe. A separate module, so their dependencies never reach the root
// module; the replace lines keep them on the code of this repository.
module github.com/eugeneliukindev/gashe/examples

go 1.25.0

require (
	github.com/eugeneliukindev/gashe v0.5.0
	github.com/eugeneliukindev/gashe/backend/redis v0.2.0
	github.com/eugeneliukindev/gashe/codec/msgpack v0.1.0
	github.com/eugeneliukindev/gashe/compress/zstd v0.1.0
	github.com/jackc/pgx/v5 v5.7.5
	github.com/prometheus/client_golang v1.22.0
	github.com/redis/go-redis/v9 v9.18.0
	go.opentelemetry.io/otel v1.35.0
	go.opentelemetry.io/otel/metric v1.35.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/go-logr/logr v1.4.2 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.2 // indirect
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.1 // indirect
	github.com/prometheus/common v0.62.0 // indirect
	github.com/prometheus/procfs v0.15.1 // indirect
	github.com/vmihailenco/msgpack/v5 v5.0.0 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	go.opentelemetry.io/auto/sdk v1.1.0 // indirect
	go.opentelemetry.io/otel/trace v1.35.0 // indirect
	golang.org/x/crypto v0.52.0 // indirect
	golang.org/x/sync v0.20.0 // indirect
	golang.org/x/sys v0.45.0 // indirect
	golang.org/x/text v0.37.0 // indirect
	google.golang.org/protobuf v1.36.5 // indirect
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

replace (
	github.com/eugeneliukindev/gashe => ../
	github.com/eugeneliukindev/gashe/backend/redis => ../backend/redis
	github.com/eugeneliukindev/gashe/codec/msgpack => ../codec/msgpack
	github.com/eugeneliukindev/gashe/compress/zstd => ../compress/zstd
)
