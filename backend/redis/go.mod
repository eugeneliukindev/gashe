module github.com/eugeneliukindev/gashe/backend/redis

go 1.25.0

require (
	github.com/eugeneliukindev/gashe v0.5.0
	github.com/redis/go-redis/v9 v9.18.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
	github.com/stretchr/testify v1.6.1 // indirect
	go.uber.org/atomic v1.11.0 // indirect
)

// The API of gashe changed before v0.3.0 in ways that break code written for these versions.
retract (
	v0.2.0
	v0.1.0
)
