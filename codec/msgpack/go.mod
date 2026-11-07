module github.com/eugeneliukindev/gashe/codec/msgpack

go 1.23

require (
	github.com/eugeneliukindev/gashe v0.5.0
	github.com/vmihailenco/msgpack/v5 v5.4.1
)

require github.com/vmihailenco/tagparser/v2 v2.0.0 // indirect

// The API of gashe changed before v0.3.0 in ways that break code written for this version.
retract v0.1.0
