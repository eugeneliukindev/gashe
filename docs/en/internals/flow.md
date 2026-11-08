# How GetOrLoad decides { #how-getorload-decides }

Every branch a call of [`GetOrLoad`](../reference/gashe.md#Cached[V].GetOrLoad) can take, with the field of
[`Policy`](../settings.md#policy) or [`Options`](../settings.md#options) that opens it. A field you leave
out keeps its branch on "no". Each end is the [`Result`](../reference/gashe.md#Result) that
[`OnResult`](../reference/gashe.md#Hooks) gets.

```mermaid
flowchart TD
    call([GetOrLoad]) --> sf{"Stampede.SingleFlight, and a call<br/>for this key already running?"}
    sf -- yes --> share([its answer, shared])
    sf -- no --> read

    subgraph Read
        read["read the key, and the counters of Tags"] --> backend{"the backend answered?<br/>Options.CircuitBreaker closed?"}
        backend -- no --> spare{"Fallback holds the key?"}
        spare -- yes --> fallback([ResultFallback])
        backend -- yes --> entry{"an entry of this type shape,<br/>stored after the last<br/>InvalidateTags of its Tags?"}
        entry -- yes --> negative{"a stored not found?"}
        negative -- "yes, within Negative.TTL" --> negativeHit([ResultNegativeHit, ErrNotFound])
        negative -- no --> fresh{"within TTL?"}
        fresh -- yes --> hit([ResultHit])
        fresh -- no --> whileRevalidate{"within Stale.WhileRevalidate?"}
        whileRevalidate -- yes --> refresh["reload in the background,<br/>one process at a time"]
        refresh --> staleWR([ResultStaleWhileRevalidate, the old result])
        whileRevalidate -- no --> ifSlow{"within Stale.IfSlow?"}
        ifSlow -- yes --> wait{"loaded within IfSlow.After?<br/>the load goes on either way"}
        wait -- no --> staleSlow([ResultStaleIfSlow, the old result])
    end

    subgraph Load
        lockFor{"Stampede.LockFor?"}
        lockFor -- yes --> lock["take the load lock: other processes<br/>wait, then read what it stored"]
        lock --> stored{"stored by the process<br/>that held the lock?"}
        stored -- yes --> lockedHit([ResultHit])
        stored -- no --> slot
        lockFor -- no --> slot{"a slot of Stampede.MaxLoads?"}
        slot -- "no, and an old result:<br/>it answers at once" --> staleOver([ResultStaleIfError or IfSlow])
        slot -- "no, until ctx ends" --> overloaded([ErrOverloaded])
        slot -- yes --> loader["call the loader"]
        loader --> failed{"it returned an error?"}
        failed -- no --> store{"Store.SlowerThan and<br/>Store.If accept it?"}
        store -- yes --> set["store it for TTL plus the Stale windows;<br/>while the backend fails, in Fallback"]
        set --> missStored([ResultMiss])
        store -- no --> missNotStored([ResultMiss, not stored])
        failed -- yes --> notFound{"Negative.IsNotFound?"}
        notFound -- yes --> setNegative["store not found<br/>for Negative.TTL"] --> missNotFound([ResultMiss, ErrNotFound])
        notFound -- no --> ifError{"an old result within Stale.IfError,<br/>and Stale.IfError.IsFailure?"}
        ifError -- yes --> staleError([ResultStaleIfError, the old result])
        ifError -- no --> loaderError([ResultMiss, the loader error])
    end

    entry -- no --> lockFor
    negative -- "yes, expired" --> lockFor
    spare -- no --> slot
    ifSlow -- "no, within Stale.IfError" --> slot
    wait -- yes --> failed
```

- **Read:** one backend call answers most calls. Only a miss, an entry past `TTL` outside
  `WhileRevalidate`, or a backend that is down reach Load.
- **Load:** `LockFor` applies to a missing key only; an old result within `IfError` or `IfSlow` is
  reloaded without the lock, since it can answer if the load fails.
- An entry outside every window is gone from the backend: the backend keeps an entry for `TTL`
  plus its longest `Stale` window, so such a call is a miss.

The fields and their defaults are in [Settings](../settings.md); what each branch stores is in
[How gashe stores data](storage.md).
