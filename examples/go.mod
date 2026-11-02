// Runnable examples of gashe. A separate module, so their dependencies never reach the root
// module; the replace lines keep them on the code of this repository.
module github.com/eugeneliukindev/gashe/examples

go 1.23

require (
	github.com/eugeneliukindev/gashe v0.2.0
	github.com/eugeneliukindev/gashe/backend/redis v0.2.0
	github.com/redis/go-redis/v9 v9.0.0
)

require (
	github.com/cespare/xxhash/v2 v2.2.0 // indirect
	github.com/dgryski/go-rendezvous v0.0.0-20200823014737-9f7001d12a5f // indirect
)

replace (
	github.com/eugeneliukindev/gashe => ../
	github.com/eugeneliukindev/gashe/backend/redis => ../backend/redis
)
