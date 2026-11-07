# Negative caching { #negative }

This page shows how you cache a "not found", so lookups of missing ids stop reaching the source.
In this section, we will go through [`Negative`](#negative-cache-not-found).

## Caching "not found" { #negative-cache-not-found }

Clients ask again and again for an id that does not exist, and every lookup reaches the database.
With [`Negative`](../reference/gashe.md#Negative), you cache the "not found" for a short [`Negative.TTL`](../reference/gashe.md#Negative), and every call gets
[`gashe.ErrNotFound`](../reference/gashe.md#ErrNotFound) meanwhile, without a query.

```go hl_lines="17-20"
--8<-- "examples/docs/negative/policy/main.go"
```

Here the first `GetOrLoad("999")` stores the "not found", lookups within 30 seconds get it without a
query, and after that the loader runs again:

```mermaid
sequenceDiagram
    participant A as GetOrLoad("999")
    participant K as Backend
    participant DB as Database
    A->>K: read user 999: nothing stored
    A->>DB: loader: one query
    DB-->>A: sql.ErrNoRows
    A->>K: store "not found" for 30s
    Note over A: returns ErrNotFound wrapping sql.ErrNoRows
    A->>K: t = 10s: read user 999
    K-->>A: "not found": ErrNotFound, no query
    Note over A,DB: after 30s the "not found" expires and the loader runs again
```

- The call gets [`gashe.ErrNotFound`](../reference/gashe.md#ErrNotFound): wrapping the loader error when it loaded, so
  `errors.Is(err, sql.ErrNoRows)` holds too, and alone while the "not found" is cached. Check it
  with `errors.Is(err, gashe.ErrNotFound)`.
- [`GetOrLoadMany`](../reference/gashe.md#GetOrLoadMany) does not support it and returns [`ErrInvalidOptions`](../reference/gashe.md#variables); [`httpcache.New`](../reference/httpcache.md#New) panics.
- With [`Stale.IfError`](../reference/gashe.md#Stale), a "not found" is an answer, never a failure, whatever `IsFailure` says: it is returned and stored, never
  hidden by the stale result.
- With [`Stampede.LockFor`](../reference/gashe.md#Stampede), processes that waited for the lock take the "not found" the holder stored.
- [`Reload`](../reference/gashe.md#Reload) that gets a "not found" leaves the stored result alone, as any error does.

!!! tip
    Make `If` match only the error that means "does not exist": every other error stays a failure,
    which [`Stale.IfError`](../reference/gashe.md#Stale) may answer with the stale result.

!!! warning
    Data created meanwhile stays "not found" until [`Negative.TTL`](../reference/gashe.md#Negative) ends: keep it short, or delete the
    key when you create the data.

??? note "Technical details"
    - The "not found" is an entry of its own with `neg` set, stored for [`Negative.TTL`](../reference/gashe.md#Negative), see
      [the GetOrLoad entry](../internals/storage.md#the-getorload-entry).

Three lookups of a user that does not exist. The two versions differ in the highlighted lines.

=== "With Negative"

    ```go hl_lines="22-25"
    --8<-- "examples/negative/with/main.go"
    ```

    The first lookup caches the "not found", the other two get it from the cache: 1 query.

    ```text title="In the backend"
    --8<-- "docs/snippets/backend/negative-with.txt"
    ```

=== "Without Negative"

    ```go
    --8<-- "examples/negative/without/main.go"
    ```

    Every lookup reaches the database, 3 queries.

    ```text title="In the backend"
    --8<-- "docs/snippets/backend/negative-without.txt"
    ```
