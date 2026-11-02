package gashe_test

import (
	"context"
	"errors"
	"slices"
	"strconv"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

func TestSetGet(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	if err := c.Set(ctx, "user:1", user{Name: "Ann"}, time.Hour); err != nil {
		t.Fatal(err)
	}
	var got user
	if err := c.Get(ctx, "user:1", &got); err != nil {
		t.Fatal(err)
	}
	if got.Name != "Ann" {
		t.Fatalf("got %+v", got)
	}
	if err := c.Get(ctx, "user:2", &got); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("miss: got %v, want ErrNotFound", err)
	}
}

func TestSetRejectsNegativeTTL(t *testing.T) {
	c := newCache(t)
	if err := c.Set(context.Background(), "k", 1, -time.Second); !errors.Is(err, gashe.ErrInvalidTTL) {
		t.Fatalf("got %v, want ErrInvalidTTL", err)
	}
}

func TestSetIfAbsentSetIfPresent(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	if ok, err := c.SetIfPresent(ctx, "k", 1, 0); err != nil || ok {
		t.Fatalf("SetIfPresent on a missing key: %v %v", ok, err)
	}
	if ok, err := c.SetIfAbsent(ctx, "k", 1, 0); err != nil || !ok {
		t.Fatalf("SetIfAbsent on a missing key: %v %v", ok, err)
	}
	if ok, err := c.SetIfAbsent(ctx, "k", 2, 0); err != nil || ok {
		t.Fatalf("SetIfAbsent on an existing key: %v %v", ok, err)
	}
	if ok, err := c.SetIfPresent(ctx, "k", 3, 0); err != nil || !ok {
		t.Fatalf("SetIfPresent on an existing key: %v %v", ok, err)
	}
	var n int
	if err := c.Get(ctx, "k", &n); err != nil || n != 3 {
		t.Fatalf("got %d %v, want 3", n, err)
	}
}

func TestIncrReadsBackWithGet(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	if err := c.Set(ctx, "n", 90, 0); err != nil {
		t.Fatal(err)
	}
	got, err := c.Incr(ctx, "n", 10, 0)
	if err != nil || got != 100 {
		t.Fatalf("Incr: %d %v", got, err)
	}
	var n int
	if err := c.Get(ctx, "n", &n); err != nil || n != 100 {
		t.Fatalf("Get after Incr: %d %v", n, err)
	}
	if err := c.Set(ctx, "u", user{Name: "Ann"}, 0); err != nil {
		t.Fatal(err)
	}
	if _, err := c.Incr(ctx, "u", 1, 0); !errors.Is(err, gashe.ErrWrongType) {
		t.Fatalf("Incr on JSON: got %v, want ErrWrongType", err)
	}
}

func TestGetManySetMany(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)

	if err := c.SetMany(ctx, map[string]any{"a": 1, "b": 2}, time.Hour); err != nil {
		t.Fatal(err)
	}
	values, err := c.GetMany(ctx, "a", "missing", "b")
	if err != nil {
		t.Fatal(err)
	}
	if values[1].Found() {
		t.Fatal("missing key is found")
	}
	if err := values[1].Decode(new(int)); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("Decode of a missing value: %v", err)
	}
	var a, b int
	if err := values[0].Decode(&a); err != nil || a != 1 {
		t.Fatalf("a = %d %v", a, err)
	}
	if err := values[2].Decode(&b); err != nil || b != 2 {
		t.Fatalf("b = %d %v", b, err)
	}
}

func TestAllKeysDeleteMatch(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	const users = 250 // more than one All batch
	for i := range users {
		if err := c.Set(ctx, "user:"+strconv.Itoa(i), i, 0); err != nil {
			t.Fatal(err)
		}
	}
	if err := c.Set(ctx, "item:1", 1, 0); err != nil {
		t.Fatal(err)
	}

	seen := 0
	for v, err := range c.All(ctx, "user:*") {
		if err != nil {
			t.Fatal(err)
		}
		var n int
		if err := v.Decode(&n); err != nil {
			t.Error(err)
		}
		seen++
	}
	if seen != users {
		t.Fatalf("All saw %d, want %d", seen, users)
	}

	stopAfter := 0
	for _, err := range c.All(ctx, "user:*") {
		if err != nil {
			t.Fatal(err)
		}
		stopAfter++
		if stopAfter == 3 {
			break
		}
	}
	if stopAfter != 3 {
		t.Fatalf("All did not stop: %d", stopAfter)
	}

	if err := c.DeleteMatch(ctx, "user:*"); err != nil {
		t.Fatal(err)
	}
	var keys []string
	for k, err := range c.Keys(ctx, "*") {
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, k)
	}
	if !slices.Equal(keys, []string{"item:1"}) {
		t.Fatalf("keys left: %v", keys)
	}
}

func TestDelete(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	_ = c.Set(ctx, "a", 1, 0)
	_ = c.Set(ctx, "b", 1, 0)
	_ = c.Set(ctx, "c", 1, 0)

	if ok, err := c.Delete(ctx, "a"); err != nil || !ok {
		t.Fatalf("Delete: %v %v", ok, err)
	}
	if ok, _ := c.Delete(ctx, "a"); ok {
		t.Fatal("second Delete reports the key existed")
	}
	if err := c.DeleteMany(ctx, "b", "c"); err != nil {
		t.Fatal(err)
	}
	for _, k := range []string{"a", "b", "c"} {
		if ok, _ := c.Exists(ctx, k); ok {
			t.Fatalf("%s still exists", k)
		}
	}
}

func TestDecodeErrorIsWrapped(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	_ = c.Set(ctx, "k", "text", 0)
	var n int
	err := c.Get(ctx, "k", &n)
	if err == nil || errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("got %v, want a decode error", err)
	}
}

func TestExpireRejectsZero(t *testing.T) {
	c := newCache(t)
	if _, err := c.Expire(context.Background(), "k", 0); !errors.Is(err, gashe.ErrInvalidTTL) {
		t.Fatalf("got %v, want ErrInvalidTTL", err)
	}
}

func TestSetManyRejectsNegativeTTL(t *testing.T) {
	c := newCache(t)
	err := c.SetMany(context.Background(), map[string]any{"a": 1}, -time.Second)
	if !errors.Is(err, gashe.ErrInvalidTTL) {
		t.Fatalf("got %v, want ErrInvalidTTL", err)
	}
}

// vanishing drops one key between Scan and GetMany, as if it expired in between.
type vanishing struct {
	gashe.Backend

	gone string
}

func (v vanishing) GetMany(ctx context.Context, keys ...string) ([][]byte, error) {
	out, err := v.Backend.GetMany(ctx, keys...)
	for i, k := range keys {
		if k == v.gone {
			out[i] = nil
		}
	}
	return out, err
}

func TestAllSkipsKeysGoneAfterScan(t *testing.T) {
	ctx := context.Background()
	c := gashe.New(vanishing{Backend: newCache(t).Backend(), gone: "b"}, gashe.Options{})
	_ = c.Set(ctx, "a", 1, 0)
	_ = c.Set(ctx, "b", 2, 0)

	var keys []string
	for v, err := range c.All(ctx, "*") {
		if err != nil {
			t.Fatal(err)
		}
		keys = append(keys, v.Key())
	}
	if !slices.Equal(keys, []string{"a"}) {
		t.Fatalf("All yielded %v, want only a", keys)
	}
}

func TestNewPanicsOnNegativeLockPollInterval(t *testing.T) {
	defer func() {
		if recover() == nil {
			t.Fatal("no panic")
		}
	}()
	gashe.New(newCache(t).Backend(), gashe.Options{LockPollInterval: -time.Second})
}
