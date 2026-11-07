<p align="center">
  <img src="docs/en/assets/logo.svg" width="200" alt="The gashe gopher, its cheeks full of cached data">
</p>

<h1 align="center">gashe</h1>

<p align="center">Caching for Go services that run as many processes.</p>

<p align="center">
  <a href="https://eugeneliukindev.github.io/gashe/"><img src="https://img.shields.io/badge/docs-site-blue" alt="Documentation"></a>
  <a href="https://pkg.go.dev/github.com/eugeneliukindev/gashe"><img src="https://pkg.go.dev/badge/github.com/eugeneliukindev/gashe.svg" alt="Go Reference"></a>
  <a href="https://github.com/eugeneliukindev/gashe/tags"><img src="https://img.shields.io/github/v/tag/eugeneliukindev/gashe?filter=v*&label=release" alt="Release"></a>
  <a href="go.mod"><img src="https://img.shields.io/github/go-mod/go-version/eugeneliukindev/gashe" alt="Go version"></a>
  <a href="LICENSE"><img src="https://img.shields.io/github/license/eugeneliukindev/gashe" alt="License"></a>
</p>

One `*gashe.Cache` stores results of expensive calls, keeps answering when a dependency is slow or
down, and guards dependencies with locks, rate limits and circuit breakers. Each process has its
own memory; over Redis, entries, tags, locks and limits are shared by every process.

**[Documentation](https://eugeneliukindev.github.io/gashe/)**

```bash
go get github.com/eugeneliukindev/gashe               # the core: memory backend, near cache, HTTP middleware
go get github.com/eugeneliukindev/gashe/backend/redis # Redis and Valkey, on go-redis v9
go get github.com/eugeneliukindev/gashe/codec/msgpack # values in MessagePack, on vmihailenco/msgpack
go get github.com/eugeneliukindev/gashe/compress/zstd # values compressed with Zstandard, on klauspost/compress
```

Go 1.25 or newer. The core module has no third-party dependencies and no global state.

## Example

```go
package main

import (
	"context"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

type User struct {
	ID   int
	Name string
}

func main() {
	ctx := context.Background()
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	defer client.Close()
	c := gashe.New(gasheredis.New(client), gashe.Options{Prefix: "app:"})
	defer c.Shutdown(ctx)

	users := gashe.For(c, gashe.Policy[User]{
		TTL:      gashe.TTL{For: time.Hour},
		Stale:    gashe.Stale{WhileRevalidate: 10 * time.Minute}, // answer at once, reload in the background
		Stampede: gashe.Stampede{LockFor: 5 * time.Second},       // a cold key loads once for all processes
		Prefix:   "user:",
	})

	u, err := users.GetOrLoad(ctx, "42", func(ctx context.Context) (User, error) {
		return User{ID: 42, Name: "Ann"}, nil // your database query
	})
	if err != nil {
		panic(err)
	}
	fmt.Println(u.Name)
}
```

## More

Every feature, with its options, diagrams and real outputs, is in the
**[documentation](https://eugeneliukindev.github.io/gashe/)**; runnable programs are in
[examples](examples/), and the API reference is on
[pkg.go.dev](https://pkg.go.dev/github.com/eugeneliukindev/gashe).
[Changelog](CHANGELOG.md), [contributing](CONTRIBUTING.md).

Under the [MIT license](LICENSE). The gopher in the logo is based on the Go gopher by
[Renee French](https://reneefrench.blogspot.com/), licensed under
[CC BY 4.0](https://creativecommons.org/licenses/by/4.0/), and was redrawn for gashe.
