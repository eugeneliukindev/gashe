module github.com/eugeneliukindev/gashe/backend/redis

go 1.23

require (
	github.com/eugeneliukindev/gashe v0.3.0
	github.com/redis/go-redis/v9 v9.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
)

// The API of gashe changed before v0.3.0 in ways that break code written for these versions.
retract (
	v0.2.0
	v0.1.0
)
