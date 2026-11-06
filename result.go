package gashe

import (
	"strconv"
	"time"
)

// Outcome is what Options.OnResult receives about one answered call.
type Outcome struct {
	// Key is the stored key, with Policy.Prefix.
	Key string
	// Result says how the call was answered.
	Result Result
	// Load is how long the loader ran for this call; 0 when it did not run, as on a hit.
	Load time.Duration
}

// Result says how a GetOrLoad call was answered.
type Result int

const (
	// ResultHit is a fresh stored result.
	ResultHit Result = iota + 1
	// ResultMiss means the loader ran.
	ResultMiss
	// ResultStaleWhileRevalidate is a stale stored result, answered while it is reloaded in the
	// background.
	ResultStaleWhileRevalidate
	// ResultStaleIfError is a stale stored result, answered instead of a loader error.
	ResultStaleIfError
	// ResultNegativeHit is a cached negative answer.
	ResultNegativeHit
	// ResultStaleIfSlow is a stale stored result, answered because the loader took longer than
	// StaleIfSlow; the load goes on in the background.
	ResultStaleIfSlow
	// ResultFallback is a result kept in Policy.Fallback, answered while the backend fails.
	ResultFallback
)

// String returns the name of r.
func (r Result) String() string {
	switch r {
	case ResultHit:
		return "hit"
	case ResultMiss:
		return "miss"
	case ResultStaleWhileRevalidate:
		return "stale-while-revalidate"
	case ResultStaleIfError:
		return "stale-if-error"
	case ResultNegativeHit:
		return "negative-hit"
	case ResultStaleIfSlow:
		return "stale-if-slow"
	case ResultFallback:
		return "fallback"
	}
	return "Result(" + strconv.Itoa(int(r)) + ")"
}
