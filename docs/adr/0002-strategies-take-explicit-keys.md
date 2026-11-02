# Caching is a generic function over Cache, called with an explicit key and a loader

Caching is `gashe.GetOrLoad(ctx, c, key, opts, load)`, not a wrapper that turns a function into a
cached function with a key derived from its name and arguments. Function names from the runtime are
unstable for closures, so derived keys would be fragile, while an explicit key plus a closure covers
any number of arguments. It is a function, not a method, because methods with type parameters need
Go 1.27 and the module supports older releases (see ADR 0004); everything that needs no type
parameter is a method of `Cache`. Options are a plain value passed on every call, like the limits of
go-redis/redis_rate, so invalid options show up at call time as ErrInvalidOptions.
