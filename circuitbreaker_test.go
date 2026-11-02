package gashe_test

import (
	"context"
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// through runs one call with result err through the breaker and returns what the caller sees.
func through(ctx context.Context, c *gashe.Cache, o gashe.BreakerOptions, err error) error {
	done, openErr := c.CircuitBreaker(ctx, "api", o)
	if openErr != nil {
		return openErr
	}
	done(err)
	return err
}

func TestCircuitBreakerIsFailure(t *testing.T) {
	ctx := context.Background()
	c := newCache(t)
	o := gashe.BreakerOptions{
		FailurePercent: 10, Window: time.Minute, OpenFor: time.Minute,
		IsFailure: func(err error) bool { return !errors.Is(err, errAPI) },
	}
	for range 5 {
		if err := through(ctx, c, o, errAPI); !errors.Is(err, errAPI) {
			t.Fatalf("%v; errors that are not failures must not open the breaker", err)
		}
	}
}

func TestCircuitBreakerInvalid(t *testing.T) {
	c := newCache(t)
	_, err := c.CircuitBreaker(context.Background(), "api", gashe.BreakerOptions{FailurePercent: 100})
	if !errors.Is(err, gashe.ErrInvalidOptions) {
		t.Fatalf("got %v, want ErrInvalidOptions", err)
	}
}
