*[TTL]: Time to live: how long a stored result is fresh
*[near cache]: A copy in the memory of the process in front of Redis, see Backends
*[tag counter]: The generation of a tag in Redis: InvalidateTags bumps it, and entries stored with an older one are misses
*[type shape]: A hash of the fields of a type, kept with an entry: after a deploy that changes the type, old entries are misses
*[stale result]: A stored result past its TTL, still answered within a window of Policy.Stale
