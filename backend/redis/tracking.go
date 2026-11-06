package redis

import (
	"context"

	goredis "github.com/redis/go-redis/v9"
)

// invalidateChannel is where Redis sends the keys a tracking connection is told about.
const invalidateChannel = "__redis__:invalidate"

// TrackingBus carries near cache invalidations from Redis itself, with client side caching
// (CLIENT TRACKING in broadcast mode); it implements nearcache.Bus. Redis reports every change of
// a key under the prefix, whoever makes it: gashe, redis-cli or another service. Needs Redis 6 or
// Valkey; a Cluster is not supported, use Bus there.
type TrackingBus struct {
	options *goredis.Options
	prefix  string
}

// NewTrackingBus returns a TrackingBus for the keys starting with prefix; "" means every key. It
// opens a connection of its own with the options of client while Subscribe runs; the caller owns
// client.
func NewTrackingBus(client *goredis.Client, prefix string) *TrackingBus {
	return &TrackingBus{options: client.Options(), prefix: prefix}
}

// Publish does nothing: Redis reports changes by itself.
func (*TrackingBus) Publish(context.Context, []string) error { return nil }

// Subscribe calls changed with the keys Redis reports until ctx is done, and changed(nil) after
// every (re)connection, failed read and FLUSHDB. If Redis refuses tracking, it keeps trying every second, and
// local copies live up to LocalTTL.
func (b *TrackingBus) Subscribe(ctx context.Context, changed func(keys []string)) error {
	o := *b.options
	o.Protocol = 2 // invalidations come as pub/sub messages only in RESP2
	o.OnConnect = func(ctx context.Context, cn *goredis.Conn) error {
		id, err := cn.ClientID(ctx).Result()
		if err != nil {
			return wrap("client id", err)
		}
		args := []any{"CLIENT", "TRACKING", "ON", "REDIRECT", id, "BCAST"}
		if b.prefix != "" {
			args = append(args, "PREFIX", b.prefix)
		}
		return wrap("client tracking", cn.Process(ctx, goredis.NewCmd(ctx, args...)))
	}
	client := goredis.NewClient(&o)
	defer func() { _ = client.Close() }()
	listen(ctx, client.Subscribe(ctx, invalidateChannel), changed, func(m *goredis.Message) []string {
		return m.PayloadSlice
	})
	return nil
}
