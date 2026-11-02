# One caching method with refresh and fallback windows instead of separate strategies

Failover, soft, early and dynamic caching are all "store a result, call it fresh for a while, then
decide what an old result is good for", so they are one `GetOrLoad` with `RefreshWindow`,
`FallbackWindow` and `LockOnMiss` rather than separate types, following the HTTP cache model
(RFC 5861, stale-while-revalidate and stale-if-error). Separate strategies stored one value in
several keys when nested and read the backend once per layer; one entry with a fresh-until time
serves every combination in one key and one read. The miss lock is taken only on a miss, because
locking around a cache lookup would cost a lock on every hit.
