// Command codecs stores the same value with each codec.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/codec/msgpack"
	"github.com/eugeneliukindev/gashe/codec/raw"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	backend := memory.New(memory.Options{})
	defer backend.Close()

	// --8<-- [start:example]
	for _, codec := range []gashe.Codec{
		gashe.JSON{},                      // (1)!
		msgpack.Codec{},                   // (2)!
		raw.Codec{Inner: msgpack.Codec{}}, // (3)!
	} {
		c := gashe.New(backend, gashe.Options{Codec: codec})
		c.Set(ctx, "user:42", User{Name: "Ann"}, time.Hour)
	}
	// --8<-- [end:example]
}
