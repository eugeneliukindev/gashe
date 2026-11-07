# Errors { #errors }

Every error gashe returns, and every function that returns it; check them with `errors.Is`.

| Error | Returned by | When |
|---|---|---|
| [`ErrNotFound`](reference/gashe.md#ErrNotFound) | [`Cache.Get`](reference/gashe.md#Cache.Get), [`Cache.TTL`](reference/gashe.md#Cache.TTL), [`Value.Decode`](reference/gashe.md#Value.Decode) | a miss |
| [`ErrInvalidTTL`](reference/gashe.md#variables) | [`Cache.Set`](reference/gashe.md#Cache.Set), [`Cache.SetIfAbsent`](reference/gashe.md#Cache.SetIfAbsent), [`Cache.SetIfPresent`](reference/gashe.md#Cache.SetIfPresent), [`Cache.SetMany`](reference/gashe.md#Cache.SetMany), [`Cache.IncrBy`](reference/gashe.md#Cache.IncrBy) | a negative TTL |
| | [`Cache.Expire`](reference/gashe.md#Cache.Expire), [`Cache.Lock`](reference/gashe.md#Cache.Lock), [`Cache.TryLock`](reference/gashe.md#Cache.TryLock), [`Cache.WithLock`](reference/gashe.md#Cache.WithLock), [`Lease.Extend`](reference/gashe.md#Lease.Extend) | a TTL that is not positive |
| [`ErrInvalidOptions`](reference/gashe.md#variables) | [`GetOrLoad`](reference/gashe.md#GetOrLoad), [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany), [`Reload`](reference/gashe.md#Reload) | a [`Policy`](reference/gashe.md#Policy) that cannot work; for [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany) also [`Stale`](reference/gashe.md#Stale), [`Stampede.LockFor`](reference/gashe.md#Stampede) or [`Negative`](reference/gashe.md#Negative) |
| | [`Cache.RateLimit`](reference/gashe.md#Cache.RateLimit) | a [`SlidingWindow`](reference/gashe.md#SlidingWindow) or [`FixedWindow`](reference/gashe.md#FixedWindow) that cannot work |
| | [`gashe.Guard`](reference/gashe.md#Guard), [`Cache.CircuitBreaker`](reference/gashe.md#Cache.CircuitBreaker) | [`CircuitBreaker`](reference/gashe.md#CircuitBreaker) that cannot work |
| [`ErrWrongType`](reference/gashe.md#variables) | [`Cache.IncrBy`](reference/gashe.md#Cache.IncrBy) | the value is not an integer |
| | [`Cache.Get`](reference/gashe.md#Cache.Get) | a Redis key that is not a string, such as a hash |
| [`ErrLockLost`](reference/gashe.md#variables) | [`Lease.Extend`](reference/gashe.md#Lease.Extend) | the lock expired, and someone else may hold it |
| | [`Cache.WithLock`](reference/gashe.md#Cache.WithLock) | an extension found the lock gone, and the function returned nil or an error that wraps `context.Cause(ctx)`; a function that returns `ctx.Err()` gives `context.Canceled` |
| [`ErrRateLimited`](reference/gashe.md#variables) | [`Cache.RateLimit`](reference/gashe.md#Cache.RateLimit) | the call is over the limit; the error is a [`*RateLimitError`](reference/gashe.md#RateLimitError) with [`RetryAfter`](reference/gashe.md#RateLimitError) |
| [`ErrCircuitOpen`](reference/gashe.md#variables) | [`gashe.Guard`](reference/gashe.md#Guard), [`Cache.CircuitBreaker`](reference/gashe.md#Cache.CircuitBreaker) | the breaker is open |
| | every call that reaches the backend, with [`Options.CircuitBreaker`](reference/gashe.md#Options) | the breaker of the backend is open; [`GetOrLoad`](reference/gashe.md#GetOrLoad), [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany), [`Reload`](reference/gashe.md#Reload) and [`RateLimit`](reference/gashe.md#Cache.RateLimit) work around it |
| [`ErrOverloaded`](reference/gashe.md#variables) | [`GetOrLoad`](reference/gashe.md#GetOrLoad), [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany), [`Reload`](reference/gashe.md#Reload) | every slot of [`Policy.Stampede.MaxLoads`](reference/gashe.md#Policy) stayed taken until `ctx` ended; it wraps `ctx.Err()` |
| [`memory.ErrTooLarge`](reference/memory.md#ErrTooLarge) | every write to a `backend/memory`: [`Cache.Set`](reference/gashe.md#Cache.Set), [`Cache.SetMany`](reference/gashe.md#Cache.SetMany), [`Cached.Set`](reference/gashe.md#Cached[V].Set) and the others | a value larger than [`MaxBytes`](reference/memory.md#Options) alone |
| `ctx.Err()` | [`Cache.Lock`](reference/gashe.md#Cache.Lock), [`Cache.WithLock`](reference/gashe.md#Cache.WithLock) | `ctx` ended while the lock was busy |
| | [`GetOrLoad`](reference/gashe.md#GetOrLoad) | `ctx` ended while [`Stampede.LockFor`](reference/gashe.md#Stampede) waited |
| | [`Cache.Shutdown`](reference/gashe.md#Cache.Shutdown) | `ctx` ended before the background reloads |
| a backend error | the basic API ([`Cache.Set`](reference/gashe.md#Cache.Set), [`Cache.Get`](reference/gashe.md#Cache.Get), [`Cache.Delete`](reference/gashe.md#Cache.Delete), [`Cache.Keys`](reference/gashe.md#Cache.Keys), ...), [`Cache.Lock`](reference/gashe.md#Cache.Lock), [`Cache.TryLock`](reference/gashe.md#Cache.TryLock), [`Cache.WithLock`](reference/gashe.md#Cache.WithLock), [`Lease.Extend`](reference/gashe.md#Lease.Extend), [`Lease.Unlock`](reference/gashe.md#Lease.Unlock), [`Cache.IsLocked`](reference/gashe.md#Cache.IsLocked), [`Cache.InvalidateTags`](reference/gashe.md#Cache.InvalidateTags), [`Cached.Set`](reference/gashe.md#Cached[V].Set), [`Cached.Delete`](reference/gashe.md#Cached[V].Delete), [`Cache.Ping`](reference/gashe.md#Cache.Ping) | the backend failed; `backend/redis` wraps it as `gashe/redis: <op>: <error>`. A backend error of an extension in [`Cache.WithLock`](reference/gashe.md#Cache.WithLock) follows the rule of [`ErrLockLost`](reference/gashe.md#variables) and is not turned into it |
| the loader error | [`GetOrLoad`](reference/gashe.md#GetOrLoad), [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany), [`Reload`](reference/gashe.md#Reload) | as the loader returned it, unless [`Stale.IfError`](reference/gashe.md#Stale) answers it |
| [`ErrNotFound`](reference/gashe.md#ErrNotFound) | [`GetOrLoad`](reference/gashe.md#GetOrLoad) | a "not found" of [`Policy.Negative`](reference/gashe.md#Policy): wrapping the loader error when it loaded, alone while it is cached |

[`GetOrLoad`](reference/gashe.md#GetOrLoad), [`GetOrLoadMany`](reference/gashe.md#GetOrLoadMany), [`Reload`](reference/gashe.md#Reload) and [`RateLimit`](reference/gashe.md#Cache.RateLimit) never return backend errors: they work
around them and send them to [`OnError`](reference/gashe.md#Hooks).

These panic at startup instead of returning an error:

| Function | Panics on |
|---|---|
| [`gashe.New`](reference/gashe.md#New) | a nil backend, a negative [`LockPollInterval`](reference/gashe.md#Options), invalid [`Options.CircuitBreaker`](reference/gashe.md#Options) |
| [`gashe.For`](reference/gashe.md#For) | an invalid [`Policy`](reference/gashe.md#Policy), with [`ErrInvalidOptions`](reference/gashe.md#variables) |
| [`memory.New`](reference/memory.md#New) | a negative [`MaxEntries`](reference/memory.md#Options) or [`MaxBytes`](reference/memory.md#Options) |
| [`nearcache.New`](reference/nearcache.md#New) | a negative [`LocalTTL`](reference/nearcache.md#Options) |
| [`gasheredis.NewBus`](reference/redis.md#NewBus) | an empty channel |
| [`httpcache.New`](reference/httpcache.md#New) | a negative [`MaxBodyBytes`](reference/httpcache.md#Options), or [`Policy.Negative`](reference/gashe.md#Policy) |
