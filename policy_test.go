package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

var errNoRows = errors.New("no rows")

func TestGetOrLoadRejectsInvalidOptions(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	_, load := counter(user{})
	for _, o := range []gashe.Policy[user]{
		{},                  // nothing would ever be stored
		{TTL: -time.Second}, // negative duration
		{TTL: "-1h"},        // negative duration as a string
		{TTL: "1hr"},        // not a duration
		{TTL: 3600},         // an int, not a time.Duration
		{TTL: func(string) time.Duration { return time.Hour }},                           // a func of another type
		{TTL: (func(user) time.Duration)(nil)},                                           // a nil func
		{TTL: time.Hour, MaxConcurrentLoads: -1},                                         // a negative limit
		{TTL: time.Hour, Jitter: 1},                                                      // jitter of 100% or more
		{TTL: time.Hour, Negative: gashe.Negative{If: func(error) bool { return true }}}, // no Err, no TTL
	} {
		if _, err := gashe.GetOrLoad(ctx, c, "k", o, load); !errors.Is(err, gashe.ErrInvalidOptions) {
			t.Errorf("%+v: got %v, want ErrInvalidOptions", o, err)
		}
		if _, err := gashe.Reload(ctx, c, "k", o, load); !errors.Is(err, gashe.ErrInvalidOptions) {
			t.Errorf("Reload %+v: got %v, want ErrInvalidOptions", o, err)
		}
	}
}

func TestNotNil(t *testing.T) {
	var nilMap map[string]int
	var nilAny any
	cases := []struct {
		name string
		got  bool
		want bool
	}{
		{"nil pointer", gashe.NotNil[*user](nil), false},
		{"pointer", gashe.NotNil(&user{}), true},
		{"nil map", gashe.NotNil(nilMap), false},
		{"nil interface", gashe.NotNil(nilAny), false},
		{"zero int", gashe.NotNil(0), true},
		{"empty string", gashe.NotNil(""), true},
	}
	for _, tc := range cases {
		if tc.got != tc.want {
			t.Errorf("%s: got %v, want %v", tc.name, tc.got, tc.want)
		}
	}
}

func TestGetOrLoadNegative(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	users := gashe.Policy[user]{
		TTL: time.Hour,
		Negative: gashe.Negative{
			If:  func(err error) bool { return errors.Is(err, errNoRows) },
			Err: errNoRows,
			TTL: time.Minute,
		},
	}
	calls := 0
	missing := func(context.Context) (user, error) {
		calls++
		return user{}, errNoRows
	}

	for range 3 {
		if _, err := gashe.GetOrLoad(ctx, c, "user:999", users, missing); !errors.Is(err, errNoRows) {
			t.Fatalf("got %v, want errNoRows", err)
		}
	}
	if calls != 1 {
		t.Fatalf("loader ran %d times for a missing user, want 1", calls)
	}
	if _, err := gashe.GetOrLoad(ctx, c, "user:1", users, func(context.Context) (user, error) {
		return user{}, errAPI
	}); !errors.Is(err, errAPI) {
		t.Fatalf("an error Match rejects: %v, want it returned", err)
	}
	if ok, _ := c.Exists(ctx, "user:1"); ok {
		t.Fatal("an error Match rejects was stored")
	}
}

func TestGetOrLoadJitter(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.Policy[int]{TTL: time.Hour, Jitter: 0.5}
	seen := map[time.Duration]bool{}
	for i := range 20 {
		key := "k" + string(rune('a'+i))
		_, _ = gashe.GetOrLoad(ctx, c, key, o, func(context.Context) (int, error) { return i, nil })
		ttl, _ := c.TTL(ctx, key)
		if ttl < 30*time.Minute || ttl > 90*time.Minute {
			t.Fatalf("TTL %v outside 1h +-50%%", ttl)
		}
		seen[ttl.Round(time.Second)] = true
	}
	if len(seen) < 2 {
		t.Fatal("Jitter did not spread the TTLs")
	}
}

func TestStaleIfSlowNeedsStaleIfError(t *testing.T) {
	_, err := gashe.GetOrLoad(context.Background(), newCache(t), "k",
		gashe.Policy[int]{TTL: time.Minute, StaleIfSlow: time.Second},
		func(context.Context) (int, error) { return 1, nil })
	if !errors.Is(err, gashe.ErrInvalidOptions) {
		t.Fatalf("err = %v, want ErrInvalidOptions", err)
	}
}

func TestGetOrLoadTTLForms(t *testing.T) {
	ctx := context.Background()
	for name, ttl := range map[string]any{
		"duration": time.Hour,
		"string":   "1h",
		"func":     func(user) time.Duration { return time.Hour },
	} {
		c := newCache(t)
		_, load := counter(user{})
		if _, err := gashe.GetOrLoad(ctx, c, "k", gashe.Policy[user]{TTL: ttl}, load); err != nil {
			t.Fatalf("%s: %v", name, err)
		}
		if left, _ := c.TTL(ctx, "k"); left <= 59*time.Minute || left > time.Hour {
			t.Fatalf("%s: stored for %v, want 1h", name, left)
		}
	}
}
