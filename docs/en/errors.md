# Errors { #errors }

Check them with `errors.Is`.

| Error | When |
|---|---|
| `ErrNotFound` | `Get`, `TTL`, `Value.Decode` on a miss |
| `ErrInvalidTTL` | a negative TTL, or a non-positive one where a lock or `Expire` needs it |
| `ErrInvalidOptions` | a `Policy`, `Limit` or `CircuitBreakerOptions` that cannot work |
| `ErrWrongType` | `IncrBy` on a value that is not an integer |
| `ErrLockLost` | `Lease.Extend` of a lock that expired |
| `ErrRateLimited` | `RateLimit` refuses a call; the error is a `*RateLimitError` with `RetryAfter` |
| `ErrCircuitOpen` | `CircuitBreaker` or `Options.CircuitBreaker` refuses a call |
| `ErrOverloaded` | `Policy.MaxConcurrentLoads` holds a load back until `ctx` ends |


Loader errors are returned unchanged. `GetOrLoad` and `RateLimit` never return backend errors:
they work around them and send them to `OnError`. `Lock`, `TryLock`, `Extend` and
`InvalidateTags` do return them.
