// Command reload calls the loader and stores its result, whatever is stored.
package main

import (
	"context"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

type User struct{ Name string }

func main() {
	ctx := context.Background()
	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{})
	userPolicy := gashe.Policy[User]{TTL: gashe.TTL{For: time.Hour}, Prefix: "users:v2:"}
	userCache := gashe.For(c, userPolicy)
	loadUser := func(context.Context) (User, error) { return User{Name: "Ann"}, nil }

	u, err := userCache.Reload(ctx, "42", loadUser)
	_, _ = u, err
	u, err = gashe.Reload(ctx, c, "42", userPolicy, loadUser)
	_, _ = u, err
}
