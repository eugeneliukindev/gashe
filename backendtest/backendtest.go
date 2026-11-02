// Package backendtest checks that a gashe.Backend keeps the contract every strategy relies on.
//
// Call Run from a test of your backend. newBackend must return an empty backend for every
// subtest; subtests do not run in parallel, so they may share one server. The suite never waits
// for real time to pass: expiry itself is the backend's own business and is not tested here.
package backendtest

import (
	"context"
	"errors"
	"math"
	"slices"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// Run runs the contract suite against backends made by newBackend.
func Run(t *testing.T, newBackend func(t *testing.T) gashe.Backend) {
	t.Helper()
	tests := []struct {
		name string
		fn   func(t *testing.T, b gashe.Backend)
	}{
		{"GetSet", testGetSet},
		{"StoredBytesAreCopied", testCopied},
		{"GetMany", testGetMany},
		{"SetMany", testSetMany},
		{"SetIfAbsentSetIfPresent", testSetIf},
		{"SetReplacesTTL", testSetReplacesTTL},
		{"Delete", testDelete},
		{"Scan", testScan},
		{"ScanEdgeCases", testScanEdgeCases},
		{"ScanStops", testScanStops},
		{"DeleteMatch", testDeleteMatch},
		{"IncrBy", testIncrBy},
		{"IncrByTTLOnlyOnCreate", testIncrByTTL},
		{"IncrByErrors", testIncrByErrors},
		{"ExpireAndTTL", testExpireTTL},
		{"Unlock", testUnlock},
		{"ExtendLock", testExtendLock},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) { tc.fn(t, newBackend(t)) })
	}
}

func must(t *testing.T, err error) {
	t.Helper()
	if err != nil {
		t.Fatal(err)
	}
}

func testGetSet(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	must(t, b.Set(ctx, "k", []byte("v"), 0))
	got, err := b.Get(ctx, "k")
	if err != nil || string(got) != "v" {
		t.Fatalf("Get = %q, %v", got, err)
	}
	must(t, b.Set(ctx, "empty", []byte{}, 0))
	if got, err := b.Get(ctx, "empty"); err != nil || got == nil || len(got) != 0 {
		t.Fatalf("empty value: %q, %v; want a non-nil empty slice", got, err)
	}
	if _, err := b.Get(ctx, "missing"); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("miss: %v, want ErrNotFound", err)
	}
	if ok, err := b.Exists(ctx, "k"); err != nil || !ok {
		t.Fatalf("Exists = %v, %v", ok, err)
	}
	if ok, err := b.Exists(ctx, "missing"); err != nil || ok {
		t.Fatalf("Exists(missing) = %v, %v", ok, err)
	}
	must(t, b.Ping(ctx))
}

func testCopied(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	in := []byte("abc")
	must(t, b.Set(ctx, "k", in, 0))
	in[0] = 'x'
	out, err := b.Get(ctx, "k")
	must(t, err)
	out[1] = 'y'
	if again, _ := b.Get(ctx, "k"); string(again) != "abc" {
		t.Fatalf("stored value changed: %q", again)
	}
}

func testGetMany(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	must(t, b.Set(ctx, "a", []byte("1"), 0))
	must(t, b.Set(ctx, "c", []byte("3"), 0))
	got, err := b.GetMany(ctx, "a", "b", "c")
	must(t, err)
	if len(got) != 3 || string(got[0]) != "1" || got[1] != nil || string(got[2]) != "3" {
		t.Fatalf("GetMany = %q", got)
	}
	if got, err := b.GetMany(ctx); err != nil || len(got) != 0 {
		t.Fatalf("GetMany() = %q, %v", got, err)
	}
}

func testSetMany(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	must(t, b.SetMany(ctx, map[string][]byte{"a": []byte("1"), "b": []byte("2")}, time.Hour))
	for _, k := range []string{"a", "b"} {
		ttl, err := b.TTL(ctx, k)
		if err != nil || ttl <= 0 || ttl > time.Hour {
			t.Fatalf("TTL(%s) = %v, %v", k, ttl, err)
		}
	}
}

func testSetIf(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	if ok, err := b.SetIfPresent(ctx, "k", []byte("1"), 0); err != nil || ok {
		t.Fatalf("SetIfPresent(missing) = %v, %v", ok, err)
	}
	if ok, err := b.SetIfAbsent(ctx, "k", []byte("1"), 0); err != nil || !ok {
		t.Fatalf("SetIfAbsent(missing) = %v, %v", ok, err)
	}
	if ok, err := b.SetIfAbsent(ctx, "k", []byte("2"), 0); err != nil || ok {
		t.Fatalf("SetIfAbsent(existing) = %v, %v", ok, err)
	}
	if ok, err := b.SetIfPresent(ctx, "k", []byte("3"), time.Hour); err != nil || !ok {
		t.Fatalf("SetIfPresent(existing) = %v, %v", ok, err)
	}
	if got, _ := b.Get(ctx, "k"); string(got) != "3" {
		t.Fatalf("value = %q, want 3", got)
	}
	if ok, err := b.SetIfPresent(ctx, "k", []byte("4"), 0); err != nil || !ok {
		t.Fatalf("SetIfPresent(existing) = %v, %v", ok, err)
	}
	if ttl, _ := b.TTL(ctx, "k"); ttl != gashe.NoExpiry {
		t.Fatalf("SetIfPresent with ttl 0 kept TTL %v", ttl)
	}
}

func testSetReplacesTTL(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	must(t, b.Set(ctx, "k", []byte("1"), time.Hour))
	must(t, b.Set(ctx, "k", []byte("2"), 0))
	if ttl, err := b.TTL(ctx, "k"); err != nil || ttl != gashe.NoExpiry {
		t.Fatalf("TTL after Set with 0 = %v, %v; want NoExpiry", ttl, err)
	}
}

func testDelete(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c"} {
		must(t, b.Set(ctx, k, []byte("1"), 0))
	}
	if ok, err := b.Delete(ctx, "a"); err != nil || !ok {
		t.Fatalf("Delete = %v, %v", ok, err)
	}
	if ok, err := b.Delete(ctx, "a"); err != nil || ok {
		t.Fatalf("Delete(missing) = %v, %v", ok, err)
	}
	must(t, b.DeleteMany(ctx, "b", "c", "missing"))
	if left := scanAll(t, b, "*"); len(left) != 0 {
		t.Fatalf("left after deletes: %v", left)
	}
}

func scanAll(t *testing.T, b gashe.Backend, pattern string) []string {
	t.Helper()
	var keys []string
	for k, err := range b.Scan(context.Background(), pattern) {
		must(t, err)
		keys = append(keys, k)
	}
	slices.Sort(keys)
	return slices.Compact(keys) // a Redis SCAN may return a key twice
}

func testScan(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	keys := []string{"user:1", "user:22", "item:1", "user.x", "hallo", "hello", "hbllo", "a*b"}
	for _, k := range keys {
		must(t, b.Set(ctx, k, []byte("1"), 0))
	}
	cases := []struct {
		pattern string
		want    []string
	}{
		{"user:*", []string{"user:1", "user:22"}},
		{"user:?", []string{"user:1"}},
		{"user.*", []string{"user.x"}},
		{"h[ae]llo", []string{"hallo", "hello"}},
		{"h[^e]llo", []string{"hallo", "hbllo"}},
		{"h[a-b]llo", []string{"hallo", "hbllo"}},
		{`a\*b`, []string{"a*b"}},
		{"nothing*", nil},
	}
	for _, tc := range cases {
		if got := scanAll(t, b, tc.pattern); !slices.Equal(got, tc.want) {
			t.Errorf("Scan(%q) = %v, want %v", tc.pattern, got, tc.want)
		}
	}
}

// testScanEdgeCases pins glob corners where implementations tend to differ from Redis.
func testScanEdgeCases(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	cases := []struct {
		pattern string
		key     string
		want    bool
	}{
		{"**a", "a", true},
		{"h[c-a]llo", "hbllo", true}, // a reversed range still matches
		{"h[ab", "hb", true},         // an unterminated class runs to the end
		{`h[\]]`, "h]", true},
		{`ab\`, `ab\`, true},   // a trailing backslash is a literal
		{"h[a-", "h-", true},   // a dash at the end is a literal
		{"h[a-]", "h-", false}, // "a-]" is a range up to ']'
		{"a*b*c", "aXXbYY", false},
	}
	for _, tc := range cases {
		must(t, b.DeleteMatch(ctx, "*"))
		must(t, b.Set(ctx, tc.key, []byte("1"), 0))
		if got := len(scanAll(t, b, tc.pattern)) == 1; got != tc.want {
			t.Errorf("Scan(%q) over %q = %v, want %v", tc.pattern, tc.key, got, tc.want)
		}
	}
}

func testScanStops(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	for _, k := range []string{"a", "b", "c", "d"} {
		must(t, b.Set(ctx, k, []byte("1"), 0))
	}
	calls := 0
	for _, err := range b.Scan(ctx, "*") {
		must(t, err)
		calls++
		break
	}
	if calls != 1 {
		t.Fatalf("the loop ran %d times after break", calls)
	}
}

func testDeleteMatch(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	for _, k := range []string{"user:1", "user:2", "item:1"} {
		must(t, b.Set(ctx, k, []byte("1"), 0))
	}
	must(t, b.DeleteMatch(ctx, "user:*"))
	if got := scanAll(t, b, "*"); !slices.Equal(got, []string{"item:1"}) {
		t.Fatalf("left after DeleteMatch: %v", got)
	}
}

func testIncrBy(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	if n, err := b.IncrBy(ctx, "n", 5, 0); err != nil || n != 5 {
		t.Fatalf("IncrBy(new) = %d, %v", n, err)
	}
	if n, err := b.IncrBy(ctx, "n", -7, 0); err != nil || n != -2 {
		t.Fatalf("IncrBy = %d, %v", n, err)
	}
	if got, _ := b.Get(ctx, "n"); string(got) != "-2" {
		t.Fatalf("stored counter = %q, want decimal -2", got)
	}
	must(t, b.Set(ctx, "m", []byte("40"), 0))
	if n, err := b.IncrBy(ctx, "m", 2, 0); err != nil || n != 42 {
		t.Fatalf("IncrBy on a decimal value = %d, %v", n, err)
	}
}

func testIncrByTTL(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	_, err := b.IncrBy(ctx, "n", 1, time.Minute)
	must(t, err)
	_, err = b.IncrBy(ctx, "n", 1, time.Hour)
	must(t, err)
	if ttl, ttlErr := b.TTL(ctx, "n"); ttlErr != nil || ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %v, %v; want the TTL from creation", ttl, ttlErr)
	}
	_, err = b.IncrBy(ctx, "plain", 1, 0)
	must(t, err)
	if ttl, _ := b.TTL(ctx, "plain"); ttl != gashe.NoExpiry {
		t.Fatalf("IncrBy with ttl 0 set TTL %v", ttl)
	}
}

func testIncrByErrors(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	must(t, b.Set(ctx, "json", []byte(`{"a":1}`), 0))
	if _, err := b.IncrBy(ctx, "json", 1, 0); !errors.Is(err, gashe.ErrWrongType) {
		t.Fatalf("IncrBy on JSON: %v, want ErrWrongType", err)
	}
	_, err := b.IncrBy(ctx, "max", math.MaxInt64, 0)
	must(t, err)
	if _, err := b.IncrBy(ctx, "max", 1, 0); err == nil {
		t.Fatal("overflow accepted")
	}
	if got, _ := b.Get(ctx, "max"); string(got) != "9223372036854775807" {
		t.Fatalf("value changed on overflow: %q", got)
	}
}

func testExpireTTL(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	if _, err := b.TTL(ctx, "missing"); !errors.Is(err, gashe.ErrNotFound) {
		t.Fatalf("TTL(missing) = %v, want ErrNotFound", err)
	}
	must(t, b.Set(ctx, "k", []byte("1"), 0))
	if ttl, err := b.TTL(ctx, "k"); err != nil || ttl != gashe.NoExpiry {
		t.Fatalf("TTL = %v, %v; want NoExpiry", ttl, err)
	}
	if ok, err := b.Expire(ctx, "k", time.Minute); err != nil || !ok {
		t.Fatalf("Expire = %v, %v", ok, err)
	}
	if ttl, err := b.TTL(ctx, "k"); err != nil || ttl <= 0 || ttl > time.Minute {
		t.Fatalf("TTL = %v, %v", ttl, err)
	}
	if ok, err := b.Expire(ctx, "missing", time.Minute); err != nil || ok {
		t.Fatalf("Expire(missing) = %v, %v", ok, err)
	}
}

func testUnlock(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	if ok, err := b.SetIfAbsent(ctx, "lock", []byte("me"), time.Minute); err != nil || !ok {
		t.Fatalf("SetIfAbsent = %v, %v", ok, err)
	}
	if ok, err := b.Unlock(ctx, "lock", []byte("other")); err != nil || ok {
		t.Fatalf("Unlock by a non-owner = %v, %v", ok, err)
	}
	if ok, err := b.Unlock(ctx, "lock", []byte("me")); err != nil || !ok {
		t.Fatalf("Unlock by the owner = %v, %v", ok, err)
	}
	if ok, _ := b.Exists(ctx, "lock"); ok {
		t.Fatal("lock survived Unlock")
	}
	if ok, err := b.Unlock(ctx, "lock", []byte("me")); err != nil || ok {
		t.Fatalf("Unlock(missing) = %v, %v", ok, err)
	}
}

func testExtendLock(t *testing.T, b gashe.Backend) {
	ctx := context.Background()
	if ok, err := b.ExtendLock(ctx, "lock", []byte("me"), time.Hour); err != nil || ok {
		t.Fatalf("ExtendLock(missing) = %v, %v", ok, err)
	}
	must(t, b.Set(ctx, "lock", []byte("me"), time.Minute))
	if ok, err := b.ExtendLock(ctx, "lock", []byte("other"), time.Hour); err != nil || ok {
		t.Fatalf("ExtendLock by a non-owner = %v, %v", ok, err)
	}
	if ttl, _ := b.TTL(ctx, "lock"); ttl > time.Minute {
		t.Fatalf("a non-owner extended the lock to %v", ttl)
	}
	if ok, err := b.ExtendLock(ctx, "lock", []byte("me"), time.Hour); err != nil || !ok {
		t.Fatalf("ExtendLock by the owner = %v, %v", ok, err)
	}
	if ttl, _ := b.TTL(ctx, "lock"); ttl <= time.Minute || ttl > time.Hour {
		t.Fatalf("TTL after ExtendLock = %v, want about an hour", ttl)
	}
}
