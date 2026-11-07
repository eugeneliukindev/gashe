package gashe_test

import (
	"errors"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
)

// through runs one call with result err through the breaker and returns what the caller sees.
func through(c *gashe.Cache, o gashe.CircuitBreaker, err error) error {
	done, openErr := c.CircuitBreaker("api", o)
	if openErr != nil {
		return openErr
	}
	done(err)
	return err
}

func TestCircuitBreakerIsFailure(t *testing.T) {
	c := newCache(t)
	o := gashe.CircuitBreaker{
		FailurePercent: 10, Window: time.Minute, OpenFor: time.Minute,
		IsFailure: func(err error) bool { return !errors.Is(err, errAPI) },
	}
	for range 5 {
		if err := through(c, o, errAPI); !errors.Is(err, errAPI) {
			t.Fatalf("%v; errors that are not failures must not open the breaker", err)
		}
	}
}

func TestCircuitBreakerInvalid(t *testing.T) {
	c := newCache(t)
	_, err := c.CircuitBreaker("api", gashe.CircuitBreaker{FailurePercent: 100})
	if !errors.Is(err, gashe.ErrInvalidOptions) {
		t.Fatalf("got %v, want ErrInvalidOptions", err)
	}
}

func TestCircuitBreakerUsesNoBackend(t *testing.T) {
	c := gashe.New(incrDown{Backend: lockDown{Backend: newCache(t).Backend()}}, gashe.Options{})
	o := gashe.CircuitBreaker{FailurePercent: 50, Window: time.Minute, OpenFor: time.Minute}
	_ = through(c, o, errAPI)
	if err := through(c, o, nil); !errors.Is(err, gashe.ErrCircuitOpen) {
		t.Fatalf("got %v, want the breaker open without any backend", err)
	}
}

func TestGuardReportsTheCall(t *testing.T) {
	c := newCache(t)
	o := gashe.CircuitBreaker{FailurePercent: 50, Window: time.Minute, OpenFor: time.Minute}
	if _, err := gashe.Guard(c, "api", o, func() (int, error) { return 0, errAPI }); !errors.Is(err, errAPI) {
		t.Fatalf("first call: %v, want the error of the call", err)
	}
	called := false
	_, err := gashe.Guard(c, "api", o, func() (int, error) { called = true; return 1, nil })
	if !errors.Is(err, gashe.ErrCircuitOpen) || called {
		t.Fatalf("after a failure: %v, called = %v; want ErrCircuitOpen without the call", err, called)
	}
}
