# `httpcache`: whole HTTP responses { #httpcache-whole-http-responses }

Stores whole responses of a handler; a hit sends the stored status, headers and body without calling
the handler.

**Below:** [How the key is made](#how-the-key-is-made), [Headers](#headers-what-each-one-does),
[Behind a reverse proxy](#behind-a-reverse-proxy).

```go
cached := httpcache.New(c, httpcache.Options{
	Policy: gashe.Policy[httpcache.Response]{TTL: time.Minute, StaleIfError: time.Hour},
	Vary: httpcache.Vary{
		Headers: []string{"Accept-Language"},
		Query:   []string{"page", "sort"},
		Cookies: []string{"currency"},
	},
})
mux.Handle("GET /products/", cached(http.HandlerFunc(listProducts)))
```

**Default:** `GET` and `HEAD` requests without `Authorization`, `Cookie` and `Range` are cached under
their host, path and query. A `200` of at most 1 MB, without `Set-Cookie`, a `Vary` header and
`Cache-Control` `no-store`, `private` or `no-cache`, is stored for `Policy.TTL`.

What a request gets; its key is the host, the path and what `Vary` names, see
[how the key is made](#how-the-key-is-made):

| The request | The answer |
|---|---|
| another method, `Range`, `Authorization` without `Vary.User`, `Cookie` without `Vary.Cookies` or `Vary.User` | goes past the cache |
| a hit | the stored response with `ETag`, `Age` and `Cache-Status: gashe; hit`; a matching `If-None-Match` gets `304 Not Modified` |
| a miss | the handler runs once for all concurrent requests of the key, as `GET` also for `HEAD` |
| a response the [rules of its headers](#headers-what-each-one-does) do not store | is sent only to the request that made it; the other waiting requests run the handler themselves |
| a 5xx within `StaleIfError` | a loader error: the last good page, with `Cache-Status: gashe; fwd=stale; fwd-status=500` |
| held back by `Policy.MaxConcurrentLoads`, or its `ctx` deadline passes while it waits for a render | `503 Service Unavailable` |

**Options** (`httpcache.Options`):

| Field | Default | What it does |
|---|---|---|
| `Policy` | required: a `TTL` | how pages are cached, as in `GetOrLoad`; a nil `CacheIf` stores `200` only |
| `Vary` | host, path and query | which parts of a request a page depends on, and so its key |
| `MaxBodyBytes` | 1 MB | a larger body is sent but not stored |

**Vary** (`httpcache.Vary`; the host and the path are always part of the key):

| Field | Default | What it does |
|---|---|---|
| `Headers` | none | request headers whose values are part of the key; a `Vary` header naming only them is stored |
| `Query` | nil: the whole query | only these query parameters; an empty list means none |
| `Cookies` | nil: requests with `Cookie` go past the cache | only these cookies; requests with other cookies are cached too |
| `User` | nil: pages are shared | who the page is for; pages per user, and `Authorization` and `Cookie` requests are cached |
| `Custom` | none | anything else the page depends on, added at the end of the key |

- **Errors:** `New` panics on a negative `MaxBodyBytes`.

!!! warning
    Every part of a request the handler builds a page from, a cookie, `X-Forwarded-Host`,
    `Accept-Language`, must be in `Vary`, or one request's page is served to others.

- **Example:** [examples/httpserver](https://github.com/eugeneliukindev/gashe/blob/main/examples/httpserver/main.go)

??? example "examples/httpserver/main.go"

    ```go
    --8<-- "examples/httpserver/main.go"
    ```

    Output:

    ```text
    --8<-- "docs/snippets/examples/httpserver.txt"
    ```

!!! danger
    A response must be the same for everyone who gets its key: a page that depends on something
    outside `Vary` leaks to other users (web cache deception, web cache poisoning).

---

## Requests and responses { #try-it-with-curl }

The requests below go to a server with three cached pages, on `localhost:8080`; every response is
from a real run, with JSON formatted for reading.

??? example "The server: examples/httpapi/main.go"

    ```go
    --8<-- "examples/httpapi/main.go"
    ```

| Page | Policy |
|---|---|
| `/products` | `TTL` 1m, `Vary` `Accept-Language` and `?page` |
| `/report` | `TTL` 1s, `StaleIfError` 1h; the handler fails after one render |
| `/export` | `TTL` 1m, `MaxConcurrentLoads` 1; half a second to get the slot |

### The first request renders and stores { #the-first-request-renders-and-stores }

The handler runs, and `Cache-Status` says the response was stored. `ETag` is a hash of the body.

```bash
--8<-- "docs/snippets/http/miss.curl"
```

```http
--8<-- "docs/snippets/http/miss.http"
```

### The next one is a hit { #the-next-one-is-a-hit }

The handler does not run. `Age` is how long ago the page was stored.

```bash
--8<-- "docs/snippets/http/hit.curl"
```

```http
--8<-- "docs/snippets/http/hit.http"
```

### A client with the page gets 304 { #a-client-with-the-page-gets-304 }

`If-None-Match` names the `ETag` the client has, so no body is sent.

```bash
--8<-- "docs/snippets/http/not-modified.curl"
```

```http
--8<-- "docs/snippets/http/not-modified.http"
```

### A query parameter outside `Vary.Query` changes nothing { #a-query-parameter-outside-varyquery-changes-nothing }

`Vary.Query` lists `page` only, so `utm_source` is not in the key and the same entry answers.

```bash
--8<-- "docs/snippets/http/query.curl"
```

```http
--8<-- "docs/snippets/http/query.http"
```

### A header in `Vary.Headers` makes another entry { #a-header-in-varyheaders-makes-another-entry }

`Accept-Language` is in `Vary.Headers`, so the Russian page is an entry of its own.

```bash
--8<-- "docs/snippets/http/vary.curl"
```

```http
--8<-- "docs/snippets/http/vary.http"
```

### A request with a cookie goes past the cache { #a-request-with-a-cookie-goes-past-the-cache }

The page might depend on the session, so it is neither read nor stored: `fwd=request`.
`Vary.Cookies` lists the cookies a page depends on, if any.

```bash
--8<-- "docs/snippets/http/cookie.curl"
```

```http
--8<-- "docs/snippets/http/cookie.http"
```

### Other methods go past the cache { #other-methods-go-past-the-cache }

```bash
--8<-- "docs/snippets/http/method.curl"
```

```http
--8<-- "docs/snippets/http/method.http"
```

### A failing handler gets the last good page { #a-failing-handler-gets-the-last-good-page }

`/report` is fresh for a second, and `StaleIfError` keeps it for an hour. Its handler works once,
then fails:

```mermaid
sequenceDiagram
    participant C as curl
    participant H as httpcache
    participant R as report handler
    C->>H: 0s: GET /report
    H->>R: a miss
    R-->>H: 200, stored
    H-->>C: 200, stored
    C->>H: 1.5s: GET /report, stale
    H->>R: reload
    R-->>H: 500
    H-->>C: the stored page, fwd-status=500
```

```bash
--8<-- "docs/snippets/http/report.curl"
```

```http
--8<-- "docs/snippets/http/report.http"
```

```bash
--8<-- "docs/snippets/http/stale.curl"
```

```http
--8<-- "docs/snippets/http/stale.http"
```

### Over `MaxConcurrentLoads`, a request gets 503 { #over-maxconcurrentloads-a-request-gets-503 }

`/export` renders for two seconds with `MaxConcurrentLoads: 1`, and each request has half a second
to get the slot:

```mermaid
sequenceDiagram
    participant C1 as curl, id 1
    participant H as httpcache
    participant C2 as curl, id 2
    C1->>H: 0s: GET /export?id=1
    Note over H: id 1 takes the slot, renders 2s
    C2->>H: 0.2s: GET /export?id=2
    Note over H: id 2 waits for the slot
    H-->>C2: 0.7s: deadline, 503
    H-->>C1: 2s: 200
```

```bash
--8<-- "docs/snippets/http/overloaded.curl"
```

```http
--8<-- "docs/snippets/http/overloaded.http"
```

## How the key is made { #how-the-key-is-made }

```text
<host><path>?<query> h:<header>=<value> c:<cookie>=<value> u=<user> x=<custom>
```

The host and the path are always part of the key; of the rest, only what `Vary` names, so requests
that differ only in other headers, cookies or query parameters get the same page.

| What | How |
|---|---|
| Example | with `Vary{Headers: {"Accept-Language"}, Query: {"page"}, Cookies: {"currency"}}`, `/list?page=2&utm_source=mail` and `/list?page=2&utm_source=ads`, with `Accept-Language: de` and `Cookie: currency=eur; _ga=111`, are both `shop.com/list?page=2 h:Accept-Language=de c:currency=eur`: the second one is a hit |
| Encoding | query parameters are sorted, values are escaped |
| Before the key | `Options.Prefix` and `Policy.Prefix` |

---

## Headers: what each one does { #headers-what-each-one-does }

Headers of the request:

| Header | What `httpcache` does |
|---|---|
| method | only `GET` and `HEAD` are cached; `HEAD` reads what `GET` stored |
| `Host` | always part of the key, with the path; the query too, or `Vary.Query` of it |
| `Authorization` | the request goes past the cache unless `Vary.User` is set: the page may belong to who sent it |
| `Cookie` | past the cache too, unless `Vary.Cookies` lists the cookies the page depends on or `Vary.User` is set |
| `Range` | the request goes past the cache |
| `If-None-Match` | a matching `ETag` gets `304 Not Modified`, compared the weak way of [RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-13.1.2) |
| `Cache-Control`, `Pragma` | ignored on purpose: a client cannot make the handler run on every request |
| the headers in `Vary.Headers` | their values are part of the key |

Headers of the response the handler writes ([RFC 9111](https://www.rfc-editor.org/rfc/rfc9111#section-3)):

| Header | What `httpcache` does |
|---|---|
| `Cache-Control: no-store`, `private`, `no-cache` | the response is not stored |
| `Cache-Control: max-age` | ignored: freshness comes from `Policy` |
| `Set-Cookie` | the response is not stored |
| `Vary` | stored only if every header it names is in `Options.Vary`: `Vary.Headers`, `Cookie` with `Vary.Cookies` or `Vary.User`, `Authorization` with `Vary.User`; `Vary: *` never |
| `ETag` | kept; without one, a hash of the body |
| hop-by-hop, `Date`, `Content-Length` | not stored ([RFC 9110](https://www.rfc-editor.org/rfc/rfc9110#section-7.6.1)) |
| any other | stored and sent with every hit |

Headers `httpcache` adds:

| Header | Value |
|---|---|
| `ETag` | on every response |
| `Age` | on a response from the cache, fresh or stale: the seconds since it was stored |
| `Cache-Status` ([RFC 9211](https://www.rfc-editor.org/rfc/rfc9211)) | `gashe; hit`, `gashe; fwd=stale; fwd-status=500` (a stale page answered a 5xx), `gashe; fwd=uri-miss; stored`, `gashe; fwd=uri-miss` (not stored), `gashe; fwd=method` or `gashe; fwd=request` (past the cache) |

---

## Behind a reverse proxy { #behind-a-reverse-proxy }

```go
cached := httpcache.New(c, httpcache.Options{
	Policy: pages,
	Vary: httpcache.Vary{
		Headers: []string{"X-Forwarded-Host", "X-Forwarded-Proto"}, // set by your proxy, not by clients
	},
})
```

The key must hold the host the handler builds the page for.

| Proxy | `Vary.Headers` |
|---|---|
| Traefik, which keeps `Host` by default, or nginx with `proxy_set_header Host $host` | nothing: the default key is right |
| a proxy that sets `Host` to its upstream | `X-Forwarded-Host`, and `X-Forwarded-Proto` if the page has absolute links |

| What | How |
|---|---|
| `X-Forwarded-*` | trust it only where your proxy sets it and drops what the client sent |
| The client IP | never in the key, `X-Forwarded-For` or `X-Real-Ip`: every visitor would get a copy of their own |
| A random query | makes a new entry each time unless `Vary.Query` lists the parameters that matter; a random `Host` too, so bound memory with `MaxBytes` |
