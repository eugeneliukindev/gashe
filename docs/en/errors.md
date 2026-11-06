# Errors { #errors }

Every error gashe returns, and every function that returns it; check them with `errors.Is`.

| Error | Returned by | When |
|---|---|---|
| `ErrNotFound` | `c.Get`, `c.TTL`, `Value.Decode` | a miss |
| `ErrInvalidTTL` | `c.Set`, `c.SetIfAbsent`, `c.SetIfPresent`, `c.SetMany`, `c.IncrBy` | a negative TTL |
| | `c.Expire`, `c.Lock`, `c.TryLock`, `c.WithLock`, `lease.Extend` | a TTL that is not positive |
| `ErrInvalidOptions` | `GetOrLoad`, `GetOrLoadMany`, `Reload` | a `Policy` that cannot work; for `GetOrLoadMany` also `StaleWhileRevalidate`, `StaleIfError`, `LockOnMiss` or `Negative` |
| | `c.RateLimit` | a `SlidingWindow` or `FixedWindow` that cannot work |
| | `gashe.Guard`, `c.CircuitBreaker` | `CircuitBreakerOptions` that cannot work |
| `ErrWrongType` | `c.IncrBy` | the value is not an integer |
| | `c.Get` | a Redis key that is not a string, such as a hash |
| `ErrLockLost` | `lease.Extend` | the lock expired, and someone else may hold it |
| | `c.WithLock` | an extension found the lock gone, and the function returned nil or an error that wraps `context.Cause(ctx)`; a function that returns `ctx.Err()` gives `context.Canceled` |
| `ErrRateLimited` | `c.RateLimit` | the call is over the limit; the error is a `*RateLimitError` with `RetryAfter` |
| `ErrCircuitOpen` | `gashe.Guard`, `c.CircuitBreaker` | the breaker is open |
| | every call that reaches the backend, with `Options.CircuitBreaker` | the breaker of the backend is open; `GetOrLoad`, `GetOrLoadMany`, `Reload` and `RateLimit` work around it |
| `ErrOverloaded` | `GetOrLoad`, `GetOrLoadMany`, `Reload` | every slot of `Policy.MaxConcurrentLoads` stayed taken until `ctx` ended; it wraps `ctx.Err()` |
| `memory.ErrTooLarge` | every write to a `backend/memory`: `c.Set`, `c.SetMany`, `Cached.Set` and the others | a value larger than `MaxBytes` alone |
| `ctx.Err()` | `c.Lock`, `c.WithLock` | `ctx` ended while the lock was busy |
| | `GetOrLoad` | `ctx` ended while `LockOnMiss` waited |
| | `c.Shutdown` | `ctx` ended before the background reloads |
| a backend error | the basic API (`c.Set`, `c.Get`, `c.Delete`, `c.Keys`, ...), `c.Lock`, `c.TryLock`, `c.WithLock`, `lease.Extend`, `lease.Unlock`, `c.IsLocked`, `c.InvalidateTags`, `Cached.Set`, `Cached.Delete`, `c.Ping` | the backend failed; `backend/redis` wraps it as `gashe/redis: <op>: <error>`. A backend error of an extension in `c.WithLock` follows the rule of `ErrLockLost` and is not turned into it |
| the loader error | `GetOrLoad`, `GetOrLoadMany`, `Reload` | as the loader returned it, unless `StaleIfError` answers it |
| `Negative.Err` | `GetOrLoad` | a "not found" is cached |

`GetOrLoad`, `GetOrLoadMany`, `Reload` and `RateLimit` never return backend errors: they work
around them and send them to `OnError`.

These panic at startup instead of returning an error:

| Function | Panics on |
|---|---|
| `gashe.New` | a nil backend, a negative `LockPollInterval`, invalid `Options.CircuitBreaker` |
| `gashe.For` | an invalid `Policy`, with `ErrInvalidOptions` |
| `memory.New` | a negative `MaxEntries` or `MaxBytes` |
| `nearcache.New` | a negative `LocalTTL` |
| `gasheredis.NewBus` | an empty channel |
| `httpcache.New` | a negative `MaxBodyBytes`, or `Policy.Negative` |
