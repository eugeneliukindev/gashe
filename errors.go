package gashe

import "errors"

var (
	// ErrNotFound reports a miss: the key has no live entry.
	ErrNotFound = errors.New("gashe: not found")
	// ErrInvalidTTL reports a TTL the operation cannot accept, such as a negative one.
	ErrInvalidTTL = errors.New("gashe: invalid ttl")
	// ErrWrongType reports an entry that holds data the operation cannot use,
	// such as IncrBy on a value that is not an integer.
	ErrWrongType = errors.New("gashe: wrong type")
	// ErrInvalidOptions reports options that cannot work, such as a negative duration.
	ErrInvalidOptions = errors.New("gashe: invalid options")
	// ErrLockLost reports a lock that expired before its holder extended it.
	ErrLockLost = errors.New("gashe: lock lost")
	// ErrRateLimited reports a call over its rate limit.
	ErrRateLimited = errors.New("gashe: rate limited")
	// ErrCircuitOpen reports a call refused by an open circuit breaker.
	ErrCircuitOpen = errors.New("gashe: circuit open")
	// ErrOverloaded reports a load refused by Stampede.MaxLoads: every slot was taken
	// until ctx ended, or an old result could answer in its place.
	ErrOverloaded = errors.New("gashe: overloaded")
)
