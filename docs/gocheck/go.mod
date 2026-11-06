// The Go blocks of the documentation are compiled here by `just docs-check` (docs/build.py): a
// module of its own, never released, so the third-party packages of the recipes stay out of every
// other module; the replace lines build the blocks against the code of this repository.
module github.com/eugeneliukindev/gashe/docs/gocheck

go 1.26.0

replace (
	github.com/eugeneliukindev/gashe => ../..
	github.com/eugeneliukindev/gashe/backend/redis => ../../backend/redis
	github.com/eugeneliukindev/gashe/codec/msgpack => ../../codec/msgpack
	github.com/eugeneliukindev/gashe/compress/zstd => ../../compress/zstd
)

require (
	github.com/eugeneliukindev/gashe v0.4.0
	github.com/eugeneliukindev/gashe/backend/redis v0.0.0-00010101000000-000000000000
	github.com/eugeneliukindev/gashe/codec/msgpack v0.0.0-00010101000000-000000000000
	github.com/eugeneliukindev/gashe/compress/zstd v0.0.0-00010101000000-000000000000
	github.com/jackc/pgx/v5 v5.12.0
	github.com/prometheus/client_golang v1.25.0
	github.com/redis/go-redis/v9 v9.23.0
	go.opentelemetry.io/otel v1.47.0
	go.opentelemetry.io/otel/metric v1.47.0
)

require (
	github.com/beorn7/perks v1.0.1 // indirect
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/go-logr/logr v1.4.4 // indirect
	github.com/go-logr/stdr v1.2.2 // indirect
	github.com/jackc/pgpassfile v1.0.0 // indirect
	github.com/jackc/pgservicefile v0.0.0-20240606120523-5a60cdf6a761 // indirect
	github.com/jackc/puddle/v2 v2.2.3 // indirect
	github.com/klauspost/compress v1.20.1 // indirect
	github.com/munnerz/goautoneg v0.0.0-20191010083416-a7dc8b61c822 // indirect
	github.com/prometheus/client_model v0.6.3 // indirect
	github.com/prometheus/common v0.72.0 // indirect
	github.com/prometheus/procfs v0.22.0 // indirect
	github.com/vmihailenco/msgpack/v5 v5.0.0 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
	go.opentelemetry.io/auto/sdk v1.2.1 // indirect
	go.opentelemetry.io/otel/log v1.47.0 // indirect
	go.opentelemetry.io/otel/trace v1.47.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sync v0.23.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
	golang.org/x/text v0.42.0 // indirect
	google.golang.org/protobuf v1.36.12 // indirect
)
