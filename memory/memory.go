// Package memory is a gashe.Backend that lives in the memory of this process: a fixed-size LRU
// with per-entry TTL. Other processes do not see its entries.
package memory

import (
	"bytes"
	"container/list"
	"context"
	"errors"
	"iter"
	"math"
	"strconv"
	"sync"
	"time"

	"github.com/eugeneliukindev/gashe"
)

const (
	defaultMaxEntries    = 1000
	defaultPurgeInterval = time.Second
)

var errOverflow = errors.New("memory: increment would overflow")

// Options configures a memory Backend. The zero value is ready to use.
type Options struct {
	// MaxEntries is the maximum number of entries, not bytes; the least recently used one is
	// evicted when it is exceeded. 0 means 1000.
	MaxEntries int
	// PurgeInterval is how often expired entries are purged in the background.
	// 0 means one second; a negative value turns the purge off. Expired entries are never
	// returned either way.
	PurgeInterval time.Duration
}

// Backend is an in-process gashe.Backend. Create it with New and stop it with Close.
type Backend struct {
	mu    sync.Mutex
	size  int
	items map[string]*list.Element
	lru   *list.List // front is the most recently used entry

	stop      chan struct{}
	done      chan struct{}
	closeOnce sync.Once
}

type entry struct {
	key      string
	data     []byte
	expireAt time.Time // zero means no expiry
}

var _ gashe.Backend = (*Backend)(nil)

// New returns a memory Backend and starts its background purge. It panics on a negative MaxEntries.
func New(o Options) *Backend {
	if o.MaxEntries < 0 {
		panic("memory: negative MaxEntries")
	}
	if o.MaxEntries == 0 {
		o.MaxEntries = defaultMaxEntries
	}
	if o.PurgeInterval == 0 {
		o.PurgeInterval = defaultPurgeInterval
	}
	m := &Backend{
		size:  o.MaxEntries,
		items: make(map[string]*list.Element),
		lru:   list.New(),
		stop:  make(chan struct{}),
		done:  make(chan struct{}),
	}
	if o.PurgeInterval < 0 {
		close(m.done)
		return m
	}
	go m.purgeEvery(o.PurgeInterval)
	return m
}

// Close stops the background purge. Entries stay readable. It is safe to call more than once.
func (m *Backend) Close() error {
	m.closeOnce.Do(func() { close(m.stop) })
	<-m.done
	return nil
}

// Get implements gashe.Backend.
func (m *Backend) Get(_ context.Context, key string) ([]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(key)
	if e == nil {
		return nil, gashe.ErrNotFound
	}
	return bytes.Clone(e.data), nil
}

// GetMany implements gashe.Backend.
func (m *Backend) GetMany(_ context.Context, keys ...string) ([][]byte, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	out := make([][]byte, len(keys))
	for i, key := range keys {
		if e := m.live(key); e != nil {
			out[i] = bytes.Clone(e.data)
		}
	}
	return out, nil
}

// Set implements gashe.Backend.
func (m *Backend) Set(_ context.Context, key string, value []byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.put(key, bytes.Clone(value), expiry(ttl))
	return nil
}

// SetMany implements gashe.Backend.
func (m *Backend) SetMany(_ context.Context, values map[string][]byte, ttl time.Duration) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	at := expiry(ttl)
	for key, value := range values {
		m.put(key, bytes.Clone(value), at)
	}
	return nil
}

// SetIfAbsent implements gashe.Backend.
func (m *Backend) SetIfAbsent(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.live(key) != nil {
		return false, nil
	}
	m.put(key, bytes.Clone(value), expiry(ttl))
	return true, nil
}

// SetIfPresent implements gashe.Backend.
func (m *Backend) SetIfPresent(_ context.Context, key string, value []byte, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.live(key) == nil {
		return false, nil
	}
	m.put(key, bytes.Clone(value), expiry(ttl))
	return true, nil
}

// Delete implements gashe.Backend.
func (m *Backend) Delete(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.live(key) == nil {
		return false, nil
	}
	m.remove(m.items[key])
	return true, nil
}

// DeleteMany implements gashe.Backend.
func (m *Backend) DeleteMany(_ context.Context, keys ...string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for _, key := range keys {
		if el, ok := m.items[key]; ok {
			m.remove(el)
		}
	}
	return nil
}

// DeleteMatch implements gashe.Backend.
func (m *Backend) DeleteMatch(_ context.Context, pattern string) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	for key, el := range m.items {
		if match(pattern, key) {
			m.remove(el)
		}
	}
	return nil
}

// Scan implements gashe.Backend. It takes a snapshot of the keys first, so the loop body may
// call the backend.
func (m *Backend) Scan(_ context.Context, pattern string) iter.Seq2[string, error] {
	return func(yield func(string, error) bool) {
		m.mu.Lock()
		now := time.Now()
		var keys []string
		for key, el := range m.items {
			if e, _ := el.Value.(*entry); !expired(e, now) && match(pattern, key) {
				keys = append(keys, key)
			}
		}
		m.mu.Unlock()
		for _, key := range keys {
			if !yield(key, nil) {
				return
			}
		}
	}
}

// Exists implements gashe.Backend.
func (m *Backend) Exists(_ context.Context, key string) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.live(key) != nil, nil
}

// Incr implements gashe.Backend.
func (m *Backend) Incr(_ context.Context, key string, by int64, ttl time.Duration) (int64, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(key)
	if e == nil {
		m.put(key, strconv.AppendInt(nil, by, 10), expiry(ttl))
		return by, nil
	}
	n, err := strconv.ParseInt(string(e.data), 10, 64)
	if err != nil {
		return 0, gashe.ErrWrongType
	}
	if (by > 0 && n > math.MaxInt64-by) || (by < 0 && n < math.MinInt64-by) {
		return 0, errOverflow
	}
	n += by
	e.data = strconv.AppendInt(nil, n, 10)
	return n, nil
}

// Expire implements gashe.Backend.
func (m *Backend) Expire(_ context.Context, key string, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(key)
	if e == nil {
		return false, nil
	}
	e.expireAt = time.Now().Add(ttl)
	return true, nil
}

// TTL implements gashe.Backend.
func (m *Backend) TTL(_ context.Context, key string) (time.Duration, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(key)
	if e == nil {
		return 0, gashe.ErrNotFound
	}
	if e.expireAt.IsZero() {
		return gashe.NoExpiry, nil
	}
	return time.Until(e.expireAt), nil
}

// Unlock implements gashe.Backend.
func (m *Backend) Unlock(_ context.Context, key string, owner []byte) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(key)
	if e == nil {
		return false, nil
	}
	if !bytes.Equal(e.data, owner) {
		return false, nil
	}
	m.remove(m.items[key])
	return true, nil
}

// ExtendLock implements gashe.Backend.
func (m *Backend) ExtendLock(_ context.Context, key string, owner []byte, ttl time.Duration) (bool, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	e := m.live(key)
	if e == nil || !bytes.Equal(e.data, owner) {
		return false, nil
	}
	e.expireAt = time.Now().Add(ttl)
	return true, nil
}

// Ping implements gashe.Backend.
func (*Backend) Ping(context.Context) error { return nil }

// Len returns the number of stored entries, expired ones that are not purged yet included.
func (m *Backend) Len() int {
	m.mu.Lock()
	defer m.mu.Unlock()
	return len(m.items)
}

// Clear removes every entry. It is not part of gashe.Backend; it exists for tests.
func (m *Backend) Clear(context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.items = make(map[string]*list.Element)
	m.lru.Init()
	return nil
}

func (m *Backend) purgeEvery(interval time.Duration) {
	defer close(m.done)
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for {
		select {
		case <-m.stop:
			return
		case <-ticker.C:
			m.purge()
		}
	}
}

func (m *Backend) purge() {
	m.mu.Lock()
	defer m.mu.Unlock()
	now := time.Now()
	for _, el := range m.items {
		if e, _ := el.Value.(*entry); expired(e, now) {
			m.remove(el)
		}
	}
}

// live returns the live entry for key and marks it recently used; an expired one is removed.
func (m *Backend) live(key string) *entry {
	el, ok := m.items[key]
	if !ok {
		return nil
	}
	e, _ := el.Value.(*entry)
	if expired(e, time.Now()) {
		m.remove(el)
		return nil
	}
	m.lru.MoveToFront(el)
	return e
}

// put stores data under key, replacing any entry, and evicts the least recently used entry
// when the size is exceeded.
func (m *Backend) put(key string, data []byte, expireAt time.Time) {
	if el, ok := m.items[key]; ok {
		m.remove(el)
	}
	m.items[key] = m.lru.PushFront(&entry{key: key, data: data, expireAt: expireAt})
	if len(m.items) > m.size {
		m.remove(m.lru.Back())
	}
}

func (m *Backend) remove(el *list.Element) {
	e, _ := el.Value.(*entry)
	delete(m.items, e.key)
	m.lru.Remove(el)
}

func expired(e *entry, now time.Time) bool {
	return !e.expireAt.IsZero() && !now.Before(e.expireAt)
}

func expiry(ttl time.Duration) time.Time {
	if ttl <= 0 {
		return time.Time{}
	}
	return time.Now().Add(ttl)
}
