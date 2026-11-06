package redis

import (
	"context"
	"encoding/json"
	"fmt"
	"time"

	goredis "github.com/redis/go-redis/v9"
)

// retryDelay is how long Subscribe waits before it tries again after Redis failed.
const retryDelay = time.Second

// Bus carries near cache invalidations over Redis pub/sub; it implements nearcache.Bus. Only key
// names are sent. Pub/sub messages are broadcast to every node, so it works on a Cluster too.
type Bus struct {
	client  goredis.UniversalClient
	channel string
}

// NewBus returns a Bus on channel. Use one channel per set of processes that share the same keys.
// The caller owns client. It panics on an empty channel.
func NewBus(client goredis.UniversalClient, channel string) *Bus {
	if channel == "" {
		panic("gashe/redis: empty bus channel")
	}
	return &Bus{client: client, channel: channel}
}

// Publish sends keys to every subscriber; nil means every key.
func (b *Bus) Publish(ctx context.Context, keys []string) error {
	payload, err := json.Marshal(keys)
	if err != nil {
		return fmt.Errorf("gashe/redis: publish: %w", err)
	}
	return wrap("publish", b.client.Publish(ctx, b.channel, payload).Err())
}

// Subscribe calls changed with every published list of keys until ctx is done. After every
// (re)subscription it calls changed(nil), since messages may have been lost.
func (b *Bus) Subscribe(ctx context.Context, changed func(keys []string)) error {
	listen(ctx, b.client.Subscribe(ctx, b.channel), changed, func(m *goredis.Message) []string {
		var keys []string
		if json.Unmarshal([]byte(m.Payload), &keys) != nil {
			return nil // an unreadable message: drop everything rather than keep a stale copy
		}
		return keys
	})
	return nil
}

// listen calls changed with the keys of every message of sub until ctx is done, and changed(nil)
// after every (re)subscription and every message it cannot read. It closes sub.
func listen(
	ctx context.Context,
	sub *goredis.PubSub,
	changed func(keys []string),
	keysOf func(*goredis.Message) []string,
) {
	defer func() { _ = sub.Close() }()
	stop := context.AfterFunc(ctx, func() { _ = sub.Close() }) // Receive ignores ctx while it reads
	defer stop()
	for {
		msg, err := sub.Receive(ctx)
		if ctx.Err() != nil {
			return
		}
		if err != nil {
			if sub.Ping(ctx) == nil {
				// The connection works, so the message was unreadable, as the one of FLUSHDB in
				// go-redis: drop everything at once.
				changed(nil)
				continue
			}
			// The connection is gone: copies stay until go-redis reconnects and subscribes
			// again on the next Receive, which drops them, since messages may have been lost.
			select {
			case <-ctx.Done():
				return
			case <-time.After(retryDelay):
			}
			continue
		}
		switch m := msg.(type) {
		case *goredis.Subscription:
			changed(nil)
		case *goredis.Message:
			changed(keysOf(m))
		}
	}
}
