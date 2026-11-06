# Cache an HTTP API { #cache-an-http-api }

Cache the answers of an external HTTP API: the stale answer while it is down, a `404` for 10m.

## The code { #the-code }

`main` runs the client against a stand-in for the API, so the program runs as it is; in your code,
keep `WeatherAPI` and pass the real base URL.

```go
package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
)

var (
	ErrNoCity  = errors.New("weather: no such city")
	errAPIDown = errors.New("weather: api unavailable")
)

type Weather struct {
	City string  `json:"city"`
	Temp float64 `json:"temp"`
}

// WeatherAPI calls an external API through the cache.
type WeatherAPI struct {
	http    *http.Client
	baseURL string
	cache   gashe.Cached[Weather]
}

func NewWeatherAPI(c *gashe.Cache, baseURL string, ttl time.Duration) *WeatherAPI {
	return &WeatherAPI{
		http:    &http.Client{Timeout: 2 * time.Second}, // never wait for the API forever
		baseURL: baseURL,
		cache: gashe.For(c, gashe.Policy[Weather]{
			TTL:          ttl,
			StaleIfError: 6 * time.Hour,                                              // the API is down: the last answer for 6 hours
			StaleIf:      func(err error) bool { return errors.Is(err, errAPIDown) }, // not for ErrNoCity
			Negative: gashe.Negative{ // a 404 is asked again after 10 minutes, not on every call
				If:  func(err error) bool { return errors.Is(err, ErrNoCity) },
				Err: ErrNoCity,
				TTL: 10 * time.Minute,
			},
			Prefix: "weather:",
		}),
	}
}

func (a *WeatherAPI) Get(ctx context.Context, city string) (Weather, error) {
	return a.cache.GetOrLoad(ctx, city, func(ctx context.Context) (Weather, error) {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/weather?city="+url.QueryEscape(city), nil)
		if err != nil {
			return Weather{}, err
		}
		resp, err := a.http.Do(req)
		if err != nil {
			return Weather{}, fmt.Errorf("%w: %w", errAPIDown, err) // timeouts and refused connections
		}
		defer resp.Body.Close()
		switch {
		case resp.StatusCode == http.StatusNotFound:
			return Weather{}, ErrNoCity
		case resp.StatusCode >= 500 || resp.StatusCode == http.StatusTooManyRequests:
			return Weather{}, fmt.Errorf("%w: %s", errAPIDown, resp.Status)
		case resp.StatusCode != http.StatusOK:
			return Weather{}, fmt.Errorf("weather: %s", resp.Status)
		}
		var w Weather
		return w, json.NewDecoder(resp.Body).Decode(&w)
	})
}

func main() {
	ctx := context.Background()
	var down atomic.Bool
	var calls atomic.Int64
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { // a stand-in for the real API
		calls.Add(1)
		switch {
		case down.Load():
			w.WriteHeader(http.StatusServiceUnavailable)
		case r.URL.Query().Get("city") != "Paris":
			w.WriteHeader(http.StatusNotFound)
		default:
			_, _ = fmt.Fprint(w, `{"city":"Paris","temp":18.5}`)
		}
	}))
	defer api.Close()

	mem := memory.New(memory.Options{})
	defer mem.Close()
	c := gashe.New(mem, gashe.Options{
		OnError: func(_ context.Context, err error) { fmt.Println("  OnError:", err) },
	})
	weather := NewWeatherAPI(c, api.URL, 100*time.Millisecond) // a short TTL to see it expire

	show := func(city string) {
		w, err := weather.Get(ctx, city)
		fmt.Printf("%-8s %+v err=%v (API calls: %d)\n", city, w, err, calls.Load())
	}
	show("Paris")    // a miss: the API is called
	show("Paris")    // a hit
	show("Atlantis") // 404: ErrNoCity, cached
	show("Atlantis") // ErrNoCity again, no call
	down.Store(true)
	time.Sleep(150 * time.Millisecond) // TTL passes while the API is down
	show("Paris")                      // the API fails: the last answer
}
```

Output:

```text
Paris    {City:Paris Temp:18.5} err=<nil> (API calls: 1)
Paris    {City:Paris Temp:18.5} err=<nil> (API calls: 1)
Atlantis {City: Temp:0} err=weather: no such city (API calls: 2)
Atlantis {City: Temp:0} err=weather: no such city (API calls: 2)
  OnError: gashe: "weather:Paris" answered with an old result: weather: api unavailable: 503 Service Unavailable
Paris    {City:Paris Temp:18.5} err=<nil> (API calls: 3)
```

## How it works { #how-it-works }

The loader turns every answer of the API into a result or an error; the policy decides by the error.

| The API answers | The loader returns | The cache |
|---|---|---|
| `200` | the `Weather` | stores it for `TTL` |
| `404` | `ErrNoCity` | caches it for `Negative.TTL`: no call meanwhile; [not found](../concepts/negative.md#negative-cache-not-found) |
| `5xx`, `429`, a timeout, no connection | `errAPIDown` | the stale answer, up to `TTL + StaleIfError` old, the error to `OnError`; [stale data](../concepts/stale.md#staleiferror-staleifslow-answer-old-data-while-the-source-fails) |
| anything else | another error | returns it, stores nothing |

| Part | What it does |
|---|---|
| `StaleIf` | keeps `ErrNoCity` out of `StaleIfError`: a city that is gone is not answered with a stale result |
| `http.Client.Timeout` | bounds the call; without it a hung API holds the request |

## Variations { #variations }

| If you need | Do |
|---|---|
| to answer at once when the API is slow | `StaleIfSlow: 300 * time.Millisecond`: the stale answer after 300ms, the call goes on in the background |
| the API to set the TTL, by `Cache-Control: max-age` | put the max-age into the result and use `TTL: func(w Weather) time.Duration { return w.MaxAge }`; [TTL](../concepts/loading.md#ttl-how-long-a-result-is-fresh) |
| to stay within the rate limit of the API | [`LockOnMiss`](../concepts/stampedes.md): one call per key for all processes; [`MaxConcurrentLoads`](../concepts/stampedes.md#maxconcurrentloads-protect-the-source-from-too-many-loads-at-once) <span class="version-badge">Added in v0.5</span>: at most N calls at once per process |
| popular keys never to wait | [`StaleWhileRevalidate`](../concepts/stale.md#stalewhilerevalidate-answer-old-data-while-reloading) |
| to cache the pages of your own server instead | [`httpcache`](../concepts/httpcache.md#httpcache-whole-http-responses) |
