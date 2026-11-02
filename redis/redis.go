// Package redis is a gashe.Backend on top of a go-redis client: a single node, a Sentinel
// failover client or a Cluster.
package redis

import (
	"context"
	"errors"
	"fmt"
	"iter"
	"strings"
	"sync"
	"time"

	goredis "github.com/redis/go-redis/v9"

	"github.com/eugeneliukindev/gashe"
)

const (
	scanCount   = 100 // keys per SCAN round trip
	deleteBatch = 100 // keys per UNLINK pipeline in DeleteMatch
)

// unlockSrc deletes the key only if it is a string holding the owner.
const unlockSrc = `
if redis.call("TYPE", KEYS[1]).ok == "string" and redis.call("GET", KEYS[1]) == ARGV[1] then
	return redis.call("DEL", KEYS[1])
end
return 0`

// Backend is a gashe.Backend over a go-redis client.
type Backend struct {
	client goredis.UniversalClient
	unlock *goredis.Script
	// nodes returns the clients that hold keys: the client itself, or every Cluster master.
	nodes func(ctx context.Context) ([]goredis.Cmdable, error)
}

var _ gashe.Backend = (*Backend)(nil)

// New returns a Backend over client. The caller owns client and closes it; Backend.Close does
// nothing.
func New(client goredis.UniversalClient) *Backend {
	b := &Backend{
		client: client,
		unlock: goredis.NewScript(unlockSrc),
	}
	b.nodes = func(context.Context) ([]goredis.Cmdable, error) { return []goredis.Cmdable{client}, nil }
	if cluster, ok := client.(*goredis.ClusterClient); ok {
		b.nodes = clusterMasters(cluster)
	}
	return b
}

func clusterMasters(cluster *goredis.ClusterClient) func(ctx context.Context) ([]goredis.Cmdable, error) {
	return func(ctx context.Context) ([]goredis.Cmdable, error) {
		var mu sync.Mutex
		var nodes []goredis.Cmdable
		err := cluster.ForEachMaster(ctx, func(_ context.Context, node *goredis.Client) error {
			mu.Lock()
			nodes = append(nodes, node)
			mu.Unlock()
			return nil
		})
		return nodes, wrap("cluster nodes", err)
	}
}

// wrap turns a go-redis error into the error gashe callers check for.
func wrap(op string, err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, goredis.Nil):
		return gashe.ErrNotFound
	case strings.Contains(err.Error(), "WRONGTYPE"), strings.Contains(err.Error(), "not an integer"):
		return fmt.Errorf("gashe/redis: %s: %w: %w", op, gashe.ErrWrongType, err)
	default:
		return fmt.Errorf("gashe/redis: %s: %w", op, err)
	}
}

// Get implements gashe.Backend.
func (b *Backend) Get(ctx context.Context, key string) ([]byte, error) {
	v, err := b.client.Get(ctx, key).Bytes()
	if err != nil {
		return nil, wrap("get", err)
	}
	return nonNil(v), nil
}

// GetMany implements gashe.Backend. Keys that hold no plain value read as missing.
func (b *Backend) GetMany(ctx context.Context, keys ...string) ([][]byte, error) {
	if len(keys) == 0 {
		return [][]byte{}, nil
	}
	cmds := make([]*goredis.StringCmd, len(keys))
	// Per-command errors are checked below; a failed connection fails every command.
	_, _ = b.client.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for i, key := range keys {
			cmds[i] = p.Get(ctx, key)
		}
		return nil
	})
	out := make([][]byte, len(keys))
	for i, cmd := range cmds {
		v, cmdErr := cmd.Bytes()
		switch e := wrap("get many", cmdErr); {
		case e == nil:
			out[i] = nonNil(v)
		case errors.Is(e, gashe.ErrNotFound), errors.Is(e, gashe.ErrWrongType):
		default:
			return nil, e
		}
	}
	return out, nil
}

// Set implements gashe.Backend.
func (b *Backend) Set(ctx context.Context, key string, value []byte, ttl time.Duration) error {
	return wrap("set", b.client.Set(ctx, key, value, ttl).Err())
}

// SetMany implements gashe.Backend.
func (b *Backend) SetMany(ctx context.Context, values map[string][]byte, ttl time.Duration) error {
	if len(values) == 0 {
		return nil
	}
	_, err := b.client.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for key, value := range values {
			p.Set(ctx, key, value, ttl)
		}
		return nil
	})
	return wrap("set many", err)
}

// SetIfAbsent implements gashe.Backend.
func (b *Backend) SetIfAbsent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	ok, err := b.client.SetNX(ctx, key, value, ttl).Result()
	return ok, wrap("set if absent", err)
}

// SetIfPresent implements gashe.Backend.
func (b *Backend) SetIfPresent(ctx context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	ok, err := b.client.SetXX(ctx, key, value, ttl).Result()
	return ok, wrap("set if present", err)
}

// Delete implements gashe.Backend.
func (b *Backend) Delete(ctx context.Context, key string) (bool, error) {
	n, err := b.client.Unlink(ctx, key).Result()
	return n > 0, wrap("delete", err)
}

// DeleteMany implements gashe.Backend. Keys are removed one command each, so they may live in
// different Cluster slots.
func (b *Backend) DeleteMany(ctx context.Context, keys ...string) error {
	if len(keys) == 0 {
		return nil
	}
	_, err := b.client.Pipelined(ctx, func(p goredis.Pipeliner) error {
		for _, key := range keys {
			p.Unlink(ctx, key)
		}
		return nil
	})
	return wrap("delete many", err)
}

// DeleteMatch implements gashe.Backend.
func (b *Backend) DeleteMatch(ctx context.Context, pattern string) error {
	batch := make([]string, 0, deleteBatch)
	for key, err := range b.Scan(ctx, pattern) {
		if err != nil {
			return err
		}
		if batch = append(batch, key); len(batch) == deleteBatch {
			if err := b.DeleteMany(ctx, batch...); err != nil {
				return err
			}
			batch = batch[:0]
		}
	}
	return b.DeleteMany(ctx, batch...)
}

// Scan implements gashe.Backend. On a Cluster it scans every master in turn. Like Redis SCAN,
// it may yield a key more than once.
func (b *Backend) Scan(ctx context.Context, pattern string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		nodes, err := b.nodes(ctx)
		if err != nil {
			yield("", err)
			return
		}
		for _, node := range nodes {
			var cursor uint64
			for {
				var keys []string
				keys, cursor, err = node.Scan(ctx, cursor, pattern, scanCount).Result()
				if err != nil {
					yield("", wrap("scan", err))
					return
				}
				for _, key := range keys {
					if !yield(key, nil) {
						return
					}
				}
				if cursor == 0 {
					break
				}
			}
		}
	}
}

// Exists implements gashe.Backend.
func (b *Backend) Exists(ctx context.Context, key string) (bool, error) {
	n, err := b.client.Exists(ctx, key).Result()
	return n > 0, wrap("exists", err)
}

// Incr implements gashe.Backend. With a ttl it runs SET NX and INCRBY in one transaction, so
// the TTL is set only when the counter is created.
func (b *Backend) Incr(ctx context.Context, key string, by int64, ttl time.Duration) (int64, error) {
	if ttl <= 0 {
		n, err := b.client.IncrBy(ctx, key, by).Result()
		return n, wrap("incr", err)
	}
	var incr *goredis.IntCmd
	_, err := b.client.TxPipelined(ctx, func(p goredis.Pipeliner) error {
		p.SetNX(ctx, key, 0, ttl)
		incr = p.IncrBy(ctx, key, by)
		return nil
	})
	if err != nil {
		return 0, wrap("incr", err)
	}
	return incr.Val(), nil
}

// Expire implements gashe.Backend.
func (b *Backend) Expire(ctx context.Context, key string, ttl time.Duration) (bool, error) {
	ok, err := b.client.PExpire(ctx, key, ttl).Result()
	return ok, wrap("expire", err)
}

// TTL implements gashe.Backend.
func (b *Backend) TTL(ctx context.Context, key string) (time.Duration, error) {
	// Raw PTTL keeps -1 and -2 unambiguous across go-redis versions.
	ms, err := b.client.Do(ctx, "PTTL", key).Int64()
	if err != nil {
		return 0, wrap("ttl", err)
	}
	const noKey, noExpiry = -2, -1
	switch ms {
	case noKey:
		return 0, gashe.ErrNotFound
	case noExpiry:
		return gashe.NoExpiry, nil
	}
	return time.Duration(ms) * time.Millisecond, nil
}

// Unlock implements gashe.Backend.
func (b *Backend) Unlock(ctx context.Context, key string, owner []byte) (bool, error) {
	n, err := b.unlock.Run(ctx, b.client, []string{key}, owner).Int64()
	return n > 0, wrap("unlock", err)
}

// SetAdd implements gashe.Backend.
func (b *Backend) SetAdd(ctx context.Context, key string, ttl time.Duration, members ...string) error {
	if len(members) == 0 {
		return nil
	}
	_, err := b.client.TxPipelined(ctx, func(p goredis.Pipeliner) error {
		p.SAdd(ctx, key, toAny(members)...)
		if ttl > 0 {
			p.PExpire(ctx, key, ttl)
		}
		return nil
	})
	return wrap("set add", err)
}

// SetRemove implements gashe.Backend.
func (b *Backend) SetRemove(ctx context.Context, key string, members ...string) error {
	if len(members) == 0 {
		return nil
	}
	return wrap("set remove", b.client.SRem(ctx, key, toAny(members)...).Err())
}

// SetPop implements gashe.Backend.
func (b *Backend) SetPop(ctx context.Context, key string, count int) ([]string, error) {
	if count <= 0 {
		return nil, nil
	}
	members, err := b.client.SPopN(ctx, key, int64(count)).Result()
	if errors.Is(err, goredis.Nil) {
		return nil, nil
	}
	return members, wrap("set pop", err)
}

// Size implements gashe.Backend. It reports MEMORY USAGE, which includes Redis overhead.
func (b *Backend) Size(ctx context.Context, key string) (int64, error) {
	n, err := b.client.MemoryUsage(ctx, key).Result()
	if errors.Is(err, goredis.Nil) {
		return 0, nil
	}
	return n, wrap("size", err)
}

// KeysCount implements gashe.Backend. On a Cluster it sums every master.
func (b *Backend) KeysCount(ctx context.Context) (int64, error) {
	nodes, err := b.nodes(ctx)
	if err != nil {
		return 0, err
	}
	var total int64
	for _, node := range nodes {
		n, err := node.DBSize(ctx).Result()
		if err != nil {
			return 0, wrap("keys count", err)
		}
		total += n
	}
	return total, nil
}

// Ping implements gashe.Backend.
func (b *Backend) Ping(ctx context.Context) error {
	return wrap("ping", b.client.Ping(ctx).Err())
}

// Clear implements gashe.Backend. It runs FLUSHDB on the selected database of every node.
func (b *Backend) Clear(ctx context.Context) error {
	nodes, err := b.nodes(ctx)
	if err != nil {
		return err
	}
	for _, node := range nodes {
		if err := node.FlushDB(ctx).Err(); err != nil {
			return wrap("clear", err)
		}
	}
	return nil
}

// Close implements gashe.Backend. It does nothing: whoever created the client closes it.
func (*Backend) Close() error { return nil }

// nonNil keeps an empty stored value distinct from a missing one.
func nonNil(b []byte) []byte {
	if b == nil {
		return []byte{}
	}
	return b
}

func toAny(ss []string) []any {
	out := make([]any, len(ss))
	for i, s := range ss {
		out[i] = s
	}
	return out
}
