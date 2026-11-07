package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/url"
)

var ErrNoCity = errors.New("weather: no such city")

var errAPIDown = errors.New("weather: api unavailable")

type Weather struct {
	City string  `json:"city"`
	Temp float64 `json:"temp"`
}

func (a *WeatherAPI) Get(ctx context.Context, city string) (Weather, error) {
	return a.cache.GetOrLoad(ctx, city, func(ctx context.Context) (Weather, error) {
		req, _ := http.NewRequestWithContext(ctx, http.MethodGet, a.baseURL+"/weather?city="+url.QueryEscape(city), nil)
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
