# How GetOrLoad decides { #how-getorload-decides }

Every branch a call of [`GetOrLoad`](../reference/gashe.md#Cached[V].GetOrLoad) can take, in five
steps. Each step has a chart of its own; a box with double sides goes on to the step of that name. A
field you leave out answers "no". The fields and their defaults are in [Settings](../settings.md).

```mermaid
flowchart LR
    call([GetOrLoad]) --> share[Share] --> read[Read] --> load[Load] --> store[Store]
    read -- "backend down" --> down[Backend down] --> load
```

## Share { #share }

```mermaid
flowchart TD
    start([GetOrLoad]) --> sf{"Stampede.SingleFlight, and a call<br/>for this key and type already running?"}
    sf -- yes --> wait{"it ends before<br/>the ctx of this call?"}
    wait -- yes --> same(["its answer or its error"])
    wait -- no --> ctxErr(["the ctx error"])
    sf -- no --> next[["Read"]]
```

The shared call runs apart from the ctx of every caller; it is cancelled once every caller has left.

## Read { #read }

```mermaid
flowchart TD
    read["read the key and the counters of its Tags"] --> backend{"the backend answered?"}
    backend -- no --> down[["Backend down"]]
    backend -- yes --> entry{"an entry that decodes, of the same type shape,<br/>stored after the last InvalidateTags of its Tags?"}
    entry -- no --> miss1[["Load, a miss"]]
    entry -- yes --> negative{"a stored not found?"}
    negative -- "yes, within Negative.For" --> notFound(["ErrNotFound"])
    negative -- "yes, expired" --> miss2[["Load, a miss"]]
    negative -- no --> fresh{"within TTL?"}
    fresh -- yes --> hit(["the stored result"])
    fresh -- no --> revalidate{"within Stale.WhileRevalidate?"}
    revalidate -- yes --> background(["the old result; one process reloads<br/>in the background, unless the cache is shutting down"])
    revalidate -- no --> ifSlow{"within Stale.IfSlow?"}
    ifSlow -- no --> stale[["Load, with the old result<br/>if within Stale.IfError"]]
    ifSlow -- yes --> busy{"the key is already loading?"}
    busy -- yes --> oldNow(["the old result"])
    busy -- no --> after{"loaded within IfSlow.After?"}
    after -- no --> oldSlow(["the old result;<br/>the load stores when it ends"])
    after -- yes --> loaded[["Load answered it"]]
```

- An open `Options.CircuitBreaker` is a backend that did not answer.
- An entry that does not decode is reported to `OnError` and read as a miss.
- A result past `TTL` and its longest `Stale` window is gone from the backend: a miss.
- While the cache shuts down, `IfSlow` waits for the loader: nothing runs in the background.

## Load { #load }

```mermaid
flowchart TD
    load["load"] --> lockFor{"a miss, and Stampede.LockFor?"}
    lockFor -- yes --> lock{"the load lock taken?"}
    lock -- "no, the backend failed" --> slot
    lock -- yes --> again{"the process that held it before<br/>stored a result or a not found?"}
    again -- yes --> theirs(["that result, or ErrNotFound"])
    again -- no --> slot
    lockFor -- no --> slot{"a free slot of Stampede.MaxLoads?"}
    slot -- "no, and an old result" --> oldOver(["the old result"])
    slot -- "no, until the ctx ends" --> overloaded(["ErrOverloaded"])
    slot -- yes --> loader["call the loader"]
    loader --> failed{"it returned an error?"}
    failed -- no --> store[["Store the result"]]
    failed -- yes --> isNotFound{"Negative.IsNotFound?"}
    isNotFound -- yes --> storeNot[["Store a not found,<br/>then ErrNotFound"]]
    isNotFound -- no --> ifError{"an old result within Stale.IfError,<br/>and Stale.IfError.IsFailure?"}
    ifError -- yes --> oldError(["the old result"])
    ifError -- no --> loaderError(["the loader error"])
```

- `LockFor` locks a missing key only: an old result within `IfError` or `IfSlow` can answer if the
  load fails, so it is reloaded without the lock.
- A call waiting for the load lock gives up when its ctx ends, with the ctx error.

## Store { #store }

```mermaid
flowchart TD
    result["a result from the loader"] --> accept{"Store.If accepts it, and the load<br/>took at least Store.SlowerThan?"}
    accept -- no --> notStored(["the result, not stored"])
    accept -- yes --> ttl{"TTL.For gives a TTL that is not negative?"}
    ttl -- no --> reported(["the result, not stored;<br/>the TTL goes to OnError"])
    ttl -- yes --> set{"the backend stored it, for TTL<br/>plus the longest Stale window?"}
    set -- yes --> stored(["the result"])
    set -- no --> spare(["the result, kept in Fallback;<br/>the error goes to OnError"])
```

A result the codec cannot encode is not stored, and the error goes to `OnError`. A not found is
stored the same way, for `Negative.For`, with no `Store` rules.

## Backend down { #backend-down }

```mermaid
flowchart TD
    down["the backend failed, or Options.CircuitBreaker is open"] --> held{"Fallback holds the key?"}
    held -- "yes, a result" --> fallback(["that result"])
    held -- "yes, a not found" --> notFound(["ErrNotFound"])
    held -- no --> load[["Load, then keep in Fallback<br/>what Store accepts, for at most Fallback.MaxTTL"]]
```

Without `Fallback`, every call loads, and the result is not stored. What each step keeps in the
backend is in [How gashe stores data](storage.md).
