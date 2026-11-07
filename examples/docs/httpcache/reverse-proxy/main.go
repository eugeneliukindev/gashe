// Command reverse-proxy keys cached responses by the host a proxy forwards.
package main

import (
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func main() {
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	pages := gashe.Policy[httpcache.Response]{TTL: gashe.TTL{For: time.Minute}}

	cached := httpcache.New(c, httpcache.Options{
		Policy: pages,
		Vary: httpcache.Vary{
			Headers: []string{"X-Forwarded-Host", "X-Forwarded-Proto"}, // (1)!
		},
	})
	_ = cached
}
