package gashe

import (
	"strconv"
)

// Result says how a GetOrLoad call was answered; Options.OnResult receives it.
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
	}
	return "Result(" + strconv.Itoa(int(r)) + ")"
}
