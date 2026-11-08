// Command compressors stores the same value with each compressor.
package main

import (
	"context"
	"strings"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/compress/gzip"
	"github.com/eugeneliukindev/gashe/compress/zstd"
)

func main() {
	ctx := context.Background()
	backend := memory.New(memory.Options{})
	defer backend.Close()

	// --8<-- [start:example]
	for _, compressor := range []gashe.Compressor{
		zstd.New(), // compress/zstd: a separate module
		gzip.New(), // compress/gzip: in the core module, the standard library only
	} {
		c := gashe.New(backend, gashe.Options{Compressor: compressor})
		c.Set(ctx, "page:home", strings.Repeat("<p>hello</p>", 1000), time.Hour)
	}
	// --8<-- [end:example]
}
