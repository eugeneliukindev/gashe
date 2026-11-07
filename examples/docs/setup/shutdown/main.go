// Command shutdown stops a service in order: the HTTP server, the cache, then the backend.
package main

import (
	"context"
	"net/http"
	"os"
	"os/signal"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
	gasheredis "github.com/eugeneliukindev/gashe/backend/redis"
)

func main() {
	client := goredis.NewClient(&goredis.Options{Addr: "localhost:6379"})
	c := gashe.New(gasheredis.New(client), gashe.Options{})
	srv := &http.Server{Addr: ":8080", ReadHeaderTimeout: 5 * time.Second}
	go srv.ListenAndServe()

	stop, cancel := signal.NotifyContext(context.Background(), os.Interrupt)
	defer cancel()
	<-stop.Done() // Ctrl+C

	ctx, cancelShutdown := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelShutdown()
	_ = srv.Shutdown(ctx) // the HTTP server first
	_ = c.Shutdown(ctx)
	_ = client.Close() // the backend last
}
