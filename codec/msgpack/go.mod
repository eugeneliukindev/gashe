module github.com/eugeneliukindev/gashe/codec/msgpack

go 1.23

// Until the root module is tagged, build against the local copy.
replace github.com/eugeneliukindev/gashe => ../../

require (
	github.com/eugeneliukindev/gashe v0.0.0
	github.com/vmihailenco/msgpack/v5 v5.0.0
)

require github.com/vmihailenco/tagparser v0.1.2 // indirect
