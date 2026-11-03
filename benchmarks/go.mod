// Benchmarks of gashe. A separate module, so their dependencies never reach the released modules;
// the replace lines keep them on the code of this repository. Never released.
module github.com/eugeneliukindev/gashe/benchmarks

go 1.23

require (
	github.com/eugeneliukindev/gashe v0.3.0
	github.com/eugeneliukindev/gashe/codec/msgpack v0.3.0
	github.com/eugeneliukindev/gashe/compress/zstd v0.3.0
)

require (
	github.com/klauspost/compress v1.18.4 // indirect
	github.com/vmihailenco/msgpack/v5 v5.0.0 // indirect
	github.com/vmihailenco/tagparser v0.1.2 // indirect
)

replace (
	github.com/eugeneliukindev/gashe => ../
	github.com/eugeneliukindev/gashe/codec/msgpack => ../codec/msgpack
	github.com/eugeneliukindev/gashe/compress/zstd => ../compress/zstd
)
