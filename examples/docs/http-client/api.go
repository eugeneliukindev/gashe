package main

import (
	"errors"
	"net/http"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// WeatherAPI calls an external API through the cache.
type WeatherAPI struct {
	http    *http.Client
	baseURL string
	cache   gashe.Cached[Weather]
}

func NewWeatherAPI(c *gashe.Cache, baseURL string, ttl time.Duration) *WeatherAPI {
	return &WeatherAPI{
		http:    &http.Client{Timeout: 2 * time.Second}, // (1)!
		baseURL: baseURL,
		cache: gashe.For(c, gashe.Policy[Weather]{
			TTL: gashe.TTL{For: ttl},
			Stale: gashe.Stale{
				IfError: gashe.StaleIfError{ // (2)!
					For:       6 * time.Hour,
					IsFailure: func(err error) bool { return errors.Is(err, errAPIDown) },
				},
			},
			Negative: gashe.Negative{ // (3)!
				IsNotFound: func(err error) bool { return errors.Is(err, ErrNoCity) },
				TTL:        10 * time.Minute,
			},
			Prefix: "weather:",
		}),
	}
}
