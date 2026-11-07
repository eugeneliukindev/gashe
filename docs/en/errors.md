# Errors { #errors }

Every error gashe returns, and every function that returns it; check them with `errors.Is`.

| Error | Returned by | When |
|---|---|---|
| `ErrNotFound` | `Cache.Get`, `Cache.TTL`, `Value.Decode` | a miss |
| `ErrInvalidTTL` | `Cache.Set`, `Cache.SetIfAbsent`, `Cache.SetIfPresent`, `Cache.SetMany`, `Cache.IncrBy` | a negative TTL |
| | `Cache.Expire`, `Cache.Lock`, `Cache.TryLock`, `Cache.WithLock`, `Lease.Extend` | a TTL that is not positive |
| `ErrInvalidOptions` | `GetOrLoad`, `GetOrLoadMany`, `Reload` | a `Policy` that cannot work; for `GetOrLoadMany` also `Stale`, `Stampede.LockFor` or `Negative` |
| | `Cache.RateLimit` | a `SlidingWindow` or `FixedWindow` that cannot work |
| | `gashe.Guard`, `Cache.CircuitBreaker` | `CircuitBreaker` that cannot work |
| `ErrWrongType` | `Cache.IncrBy` | the value is not an integer |
| | `Cache.Get` | a Redis key that is not a string, such as a hash |
| `ErrLockLost` | `Lease.Extend` | the lock expired, and someone else may hold it |
| | `Cache.WithLock` | an extension found the lock gone, and the function returned nil or an error that wraps `context.Cause(ctx)`; a function that returns `ctx.Err()` gives `context.Canceled` |
| `ErrRateLimited` | `Cache.RateLimit` | the call is over the limit; the error is a `*RateLimitError` with `RetryAfter` |
| `ErrCircuitOpen` | `gashe.Guard`, `Cache.CircuitBreaker` | the breaker is open |
| | every call that reaches the backend, with `Options.CircuitBreaker` | the breaker of the backend is open; `GetOrLoad`, `GetOrLoadMany`, `Reload` and `RateLimit` work around it |
| `ErrOverloaded` | `GetOrLoad`, `GetOrLoadMany`, `Reload` | every slot of `Policy.Stampede.MaxLoads` stayed taken until `ctx` ended; it wraps `ctx.Err()` |
| `memory.ErrTooLarge` | every write to a `backend/memory`: `Cache.Set`, `Cache.SetMany`, `Cached.Set` and the others | a value larger than `MaxBytes` alone |
| `ctx.Err()` | `Cache.Lock`, `Cache.WithLock` | `ctx` ended while the lock was busy |
| | `GetOrLoad` | `ctx` ended while `Stampede.LockFor` waited |
| | `Cache.Shutdown` | `ctx` ended before the background reloads |
| a backend error | the basic API (`Cache.Set`, `Cache.Get`, `Cache.Delete`, `Cache.Keys`, ...), `Cache.Lock`, `Cache.TryLock`, `Cache.WithLock`, `Lease.Extend`, `Lease.Unlock`, `Cache.IsLocked`, `Cache.InvalidateTags`, `Cached.Set`, `Cached.Delete`, `Cache.Ping` | the backend failed; `backend/redis` wraps it as `gashe/redis: <op>: <error>`. A backend error of an extension in `Cache.WithLock` follows the rule of `ErrLockLost` and is not turned into it |
| the loader error | `GetOrLoad`, `GetOrLoadMany`, `Reload` | as the loader returned it, unless `Stale.IfError` answers it |
| `ErrNotFound` | `GetOrLoad` | a "not found" of `Policy.Negative`: wrapping the loader error when it loaded, alone while it is cached |

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
