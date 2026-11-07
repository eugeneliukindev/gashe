module github.com/eugeneliukindev/gashe/backend/redis

go 1.26.0

require (
	github.com/eugeneliukindev/gashe v0.5.0
	github.com/redis/go-redis/v9 v9.23.0
)

require (
	github.com/cespare/xxhash/v2 v2.3.0 // indirect
	go.uber.org/atomic v1.12.0 // indirect
	golang.org/x/sys v0.48.0 // indirect
)

// The API of gashe changed before v0.3.0 in ways that break code written for these versions.
retract (
	v0.2.0
	v0.1.0
)
