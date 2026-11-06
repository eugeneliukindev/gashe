package httpcache_test

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/eugeneliukindev/gashe"
	"github.com/eugeneliukindev/gashe/backend/memory"
	"github.com/eugeneliukindev/gashe/httpcache"
)

func newCache(t *testing.T) *gashe.Cache {
	t.Helper()
	mem := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = mem.Close() })
	return gashe.New(mem, gashe.Options{})
}

// page is a handler that answers body with status and the headers set by header, counting runs.
type page struct {
	runs   atomic.Int32
	status atomic.Int32
	body   string
	header func(h http.Header)
	method atomic.Value // the method of the last run
}

func (p *page) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p.runs.Add(1)
	p.method.Store(r.Method)
	if p.header != nil {
		p.header(w.Header())
	}
	if s := p.status.Load(); s != 0 {
		w.WriteHeader(int(s))
	}
	_, _ = io.WriteString(w, p.body)
}

func serve(t *testing.T, c *gashe.Cache, opts *httpcache.Options, h http.Handler) http.Handler {
	t.Helper()
	o := *opts
	if o.Policy.TTL == nil && o.Policy.StaleIfError == 0 {
		o.Policy.TTL = time.Minute
	}
	return httpcache.New(c, o)(h)
}

func get(h http.Handler, method, target string, header ...string) *httptest.ResponseRecorder {
	r := httptest.NewRequestWithContext(context.Background(), method, target, http.NoBody)
	for i := 0; i+1 < len(header); i += 2 {
		r.Header.Set(header[i], header[i+1])
	}
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	return w
}

func TestMissThenHit(t *testing.T) {
	p := &page{body: "catalog"}
	h := serve(t, newCache(t), &httpcache.Options{}, p)

	first := get(h, http.MethodGet, "/products?page=2")
	second := get(h, http.MethodGet, "/products?page=2")
	if p.runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want 1", p.runs.Load())
	}
	if first.Header().Get("Cache-Status") != "gashe; fwd=uri-miss; stored" ||
		second.Header().Get("Cache-Status") != "gashe; hit" {
		t.Fatalf("Cache-Status = %q, %q", first.Header().Get("Cache-Status"), second.Header().Get("Cache-Status"))
	}
	if second.Body.String() != "catalog" || second.Code != http.StatusOK || second.Header().Get("Age") == "" {
		t.Fatalf("hit: %d %q, Age %q", second.Code, second.Body.String(), second.Header().Get("Age"))
	}
	get(h, http.MethodGet, "/products?page=3")
	if p.runs.Load() != 2 {
		t.Fatal("another query must be another key")
	}
}

func TestIfNoneMatch(t *testing.T) {
	p := &page{body: "catalog"}
	h := serve(t, newCache(t), &httpcache.Options{}, p)

	etag := get(h, http.MethodGet, "/").Header().Get("ETag")
	if etag == "" {
		t.Fatal("no ETag")
	}
	w := get(h, http.MethodGet, "/", "If-None-Match", `"other", W/`+etag)
	if w.Code != http.StatusNotModified || w.Body.Len() != 0 {
		t.Fatalf("matching If-None-Match: %d %q, want 304 without a body", w.Code, w.Body.String())
	}
	if w := get(h, http.MethodGet, "/", "If-None-Match", `"other"`); w.Code != http.StatusOK {
		t.Fatalf("other ETag: %d, want 200", w.Code)
	}
}

func TestHandlerETagIsKept(t *testing.T) {
	p := &page{body: "x", header: func(h http.Header) { h.Set("ETag", `"v7"`) }}
	h := serve(t, newCache(t), &httpcache.Options{}, p)

	if got := get(h, http.MethodGet, "/").Header().Get("ETag"); got != `"v7"` {
		t.Fatalf("ETag = %q", got)
	}
	if w := get(h, http.MethodGet, "/", "If-None-Match", `"v7"`); w.Code != http.StatusNotModified {
		t.Fatalf("got %d, want 304", w.Code)
	}
}

func TestStaleIfErrorAnswersServerErrors(t *testing.T) {
	p := &page{body: "last good page"}
	h := serve(t, newCache(t), &httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
		StaleIfError: time.Hour, // TTL 0: the handler runs every time, its failures get the stored page
	}}, p)

	get(h, http.MethodGet, "/")
	p.status.Store(http.StatusBadGateway)
	w := get(h, http.MethodGet, "/")
	if w.Code != http.StatusOK || w.Body.String() != "last good page" || p.runs.Load() != 2 {
		t.Fatalf("during the outage: %d %q after %d runs", w.Code, w.Body.String(), p.runs.Load())
	}
	if got, want := w.Header().Get("Cache-Status"), "gashe; fwd=stale; fwd-status=502"; got != want {
		t.Fatalf("Cache-Status %q, want %q: the page is stale and the handler failed", got, want)
	}
}

func TestServerErrorIsSentAndNotStored(t *testing.T) {
	p := &page{body: "down"}
	p.status.Store(http.StatusServiceUnavailable)
	h := serve(t, newCache(t), &httpcache.Options{}, p)

	for range 2 {
		if w := get(h, http.MethodGet, "/"); w.Code != http.StatusServiceUnavailable || w.Body.String() != "down" {
			t.Fatalf("got %d %q", w.Code, w.Body.String())
		}
	}
	if p.runs.Load() != 2 {
		t.Fatalf("handler ran %d times; a 5xx must not be stored", p.runs.Load())
	}
}

func TestResponsesThatAreNotStored(t *testing.T) {
	cases := []struct {
		name   string
		status int32
		header func(h http.Header)
	}{
		{"Set-Cookie", 0, func(h http.Header) { h.Set("Set-Cookie", "session=1") }},
		{"Vary", 0, func(h http.Header) { h.Set("Vary", "Accept-Language") }},
		{"no-store", 0, func(h http.Header) { h.Set("Cache-Control", "no-store") }},
		{"private", 0, func(h http.Header) { h.Set("Cache-Control", "max-age=60, private") }},
		{"no-cache", 0, func(h http.Header) { h.Set("Cache-Control", `no-cache="Set-Cookie"`) }},
		{"404", http.StatusNotFound, nil},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &page{body: "body", header: tc.header}
			p.status.Store(tc.status)
			h := serve(t, newCache(t), &httpcache.Options{}, p)
			for range 2 {
				if w := get(h, http.MethodGet, "/"); w.Body.String() != "body" {
					t.Fatalf("body %q", w.Body.String())
				}
			}
			if p.runs.Load() != 2 {
				t.Fatalf("handler ran %d times, want 2: the response must not be stored", p.runs.Load())
			}
		})
	}
}

func TestLargeBodyIsSentWholeAndNotStored(t *testing.T) {
	p := &page{body: strings.Repeat("x", 100)}
	h := serve(t, newCache(t), &httpcache.Options{MaxBodyBytes: 10}, p)

	for range 2 {
		if w := get(h, http.MethodGet, "/"); w.Body.Len() != 100 {
			t.Fatalf("sent %d bytes, want all 100", w.Body.Len())
		}
	}
	if p.runs.Load() != 2 {
		t.Fatal("a body over MaxBodyBytes was stored")
	}
}

func TestCacheIfDecidesStatuses(t *testing.T) {
	p := &page{body: "no such product"}
	p.status.Store(http.StatusNotFound)
	h := serve(t, newCache(t), &httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
		TTL:     time.Minute,
		CacheIf: func(r httpcache.Response) bool { return r.Status == 200 || r.Status == 404 },
	}}, p)

	get(h, http.MethodGet, "/p/1")
	if w := get(h, http.MethodGet, "/p/1"); w.Code != http.StatusNotFound || p.runs.Load() != 1 {
		t.Fatalf("got %d after %d runs; CacheIf allowed 404", w.Code, p.runs.Load())
	}
}

func TestRequestsThatBypassTheCache(t *testing.T) {
	cases := []struct {
		name   string
		method string
		header []string
	}{
		{"POST", http.MethodPost, nil},
		{"Range", http.MethodGet, []string{"Range", "bytes=0-1"}},
		{"Authorization", http.MethodGet, []string{"Authorization", "Bearer t"}},
		{"Cookie", http.MethodGet, []string{"Cookie", "session=ann"}}, // a page of whoever sent it
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &page{body: "body"}
			h := serve(t, newCache(t), &httpcache.Options{}, p)
			for range 2 {
				get(h, tc.method, "/", tc.header...)
			}
			if p.runs.Load() != 2 {
				t.Fatalf("handler ran %d times, want 2", p.runs.Load())
			}
		})
	}
}

func TestVaryUserCachesPagesPerUser(t *testing.T) {
	p := &page{body: "my orders"}
	h := serve(t, newCache(t), &httpcache.Options{
		Vary: httpcache.Vary{User: func(r *http.Request) string { return r.Header.Get("Authorization") }},
	}, p)

	get(h, http.MethodGet, "/orders", "Authorization", "Bearer ann")
	get(h, http.MethodGet, "/orders", "Authorization", "Bearer ann")
	get(h, http.MethodGet, "/orders", "Authorization", "Bearer bob")
	if p.runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want one per user", p.runs.Load())
	}
}

func TestRequestCacheControlIsIgnored(t *testing.T) {
	p := &page{body: "catalog"}
	h := serve(t, newCache(t), &httpcache.Options{}, p)

	get(h, http.MethodGet, "/")
	get(h, http.MethodGet, "/", "Cache-Control", "no-cache", "Pragma", "no-cache")
	if p.runs.Load() != 1 {
		t.Fatal("a client header made the handler run again")
	}
}

func TestHead(t *testing.T) {
	p := &page{body: "catalog"}
	h := serve(t, newCache(t), &httpcache.Options{}, p)

	w := get(h, http.MethodHead, "/")
	if w.Code != http.StatusOK || w.Body.Len() != 0 || p.method.Load() != http.MethodGet {
		t.Fatalf("HEAD miss: %d, %d bytes, handler ran as %v; want 200, no body, GET",
			w.Code, w.Body.Len(), p.method.Load())
	}
	if w := get(h, http.MethodGet, "/"); w.Body.String() != "catalog" || p.runs.Load() != 1 {
		t.Fatalf("GET after HEAD: %q after %d runs; HEAD must store the GET response", w.Body.String(), p.runs.Load())
	}
	if w := get(h, http.MethodHead, "/"); w.Body.Len() != 0 || w.Header().Get("Cache-Status") != "gashe; hit" {
		t.Fatalf("HEAD hit: %d bytes, Cache-Status %q", w.Body.Len(), w.Header().Get("Cache-Status"))
	}
}

func TestStaleIfErrorDoesNotHideAnswersThatAreNotStored(t *testing.T) {
	p := &page{body: "product"}
	h := serve(t, newCache(t), &httpcache.Options{Policy: gashe.Policy[httpcache.Response]{StaleIfError: time.Hour}}, p)

	get(h, http.MethodGet, "/p/1")
	p.status.Store(http.StatusNotFound) // the product was deleted: a real answer, not a failure
	if w := get(h, http.MethodGet, "/p/1"); w.Code != http.StatusNotFound {
		t.Fatalf("got %d; a 404 must reach the client, not the stored 200", w.Code)
	}
}

func TestCacheStatusSaysWhyTheHandlerRan(t *testing.T) {
	cases := []struct {
		name   string
		method string
		header []string
		cookie bool
		want   string
	}{
		{"not stored", http.MethodGet, nil, true, "gashe; fwd=uri-miss"},
		{"method", http.MethodPost, nil, false, "gashe; fwd=method"},
		{"Range", http.MethodGet, []string{"Range", "bytes=0-1"}, false, "gashe; fwd=request"},
		{"Authorization", http.MethodGet, []string{"Authorization", "Bearer t"}, false, "gashe; fwd=request"},
		{"Cookie", http.MethodGet, []string{"Cookie", "session=ann"}, false, "gashe; fwd=request"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			p := &page{body: "x"}
			if tc.cookie {
				p.header = func(h http.Header) { h.Set("Set-Cookie", "s=1") }
			}
			h := serve(t, newCache(t), &httpcache.Options{}, p)
			if got := get(h, tc.method, "/", tc.header...).Header().Get("Cache-Status"); got != tc.want {
				t.Fatalf("Cache-Status = %q, want %q", got, tc.want)
			}
		})
	}
}

// greeting answers in the language of the request and says so in Vary.
type greeting struct {
	runs atomic.Int32
	vary string
}

func (g *greeting) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	g.runs.Add(1)
	if g.vary != "" {
		w.Header().Set("Vary", g.vary)
	}
	_, _ = io.WriteString(w, "hello in "+r.Header.Get("Accept-Language")) //nolint:gosec // test output, never rendered
}

func TestVaryHeadersSplitTheKey(t *testing.T) {
	g := &greeting{vary: "Accept-Language"}
	h := serve(t, newCache(t), &httpcache.Options{Vary: httpcache.Vary{Headers: []string{"accept-language"}}}, g)

	for _, lang := range []string{"en", "de", "en", "de"} {
		w := get(h, http.MethodGet, "/", "Accept-Language", lang)
		if got := w.Body.String(); got != "hello in "+lang {
			t.Fatalf("%s: got %q; a page in one language was served for another", lang, got)
		}
	}
	if g.runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want once per language", g.runs.Load())
	}
}

func TestVaryOutsideTheKeyIsNotStored(t *testing.T) {
	for _, vary := range []string{"Accept-Language", "*", "Accept-Language, Accept-Encoding"} {
		t.Run(vary, func(t *testing.T) {
			g := &greeting{vary: vary}
			headers := []string{"Accept-Encoding"}
			if vary == "Accept-Language" {
				headers = nil
			}
			h := serve(t, newCache(t), &httpcache.Options{Vary: httpcache.Vary{Headers: headers}}, g)
			get(h, http.MethodGet, "/", "Accept-Language", "en")
			get(h, http.MethodGet, "/", "Accept-Language", "de")
			if g.runs.Load() != 2 {
				t.Fatalf("handler ran %d times: a response that varies on a header outside the key was shared",
					g.runs.Load())
			}
		})
	}
}

// echo answers with its query and its cookies, so a test sees which request rendered a page.
type echo struct {
	runs atomic.Int32
}

func (e *echo) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	e.runs.Add(1)
	_, _ = io.WriteString(w, r.URL.RawQuery+" "+r.Header.Get("Cookie")) //nolint:gosec // test output, never rendered
}

func TestVaryQueryKeepsOnlyListedParameters(t *testing.T) {
	e := &echo{}
	h := serve(t, newCache(t), &httpcache.Options{Vary: httpcache.Vary{Query: []string{"page", "sort"}}}, e)

	get(h, http.MethodGet, "/list?page=2&sort=name")
	get(h, http.MethodGet, "/list?sort=name&page=2&utm_source=mail") // the same page: order and utm ignored
	get(h, http.MethodGet, "/list?page=3&sort=name")
	if e.runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want one per page", e.runs.Load())
	}
}

func TestVaryCookiesCachesRequestsWithOtherCookies(t *testing.T) {
	e := &echo{}
	h := serve(t, newCache(t), &httpcache.Options{Vary: httpcache.Vary{Cookies: []string{"currency"}}}, e)

	get(h, http.MethodGet, "/", "Cookie", "currency=usd; _ga=1")
	get(h, http.MethodGet, "/", "Cookie", "currency=usd; _ga=2") // an analytics cookie does not split the key
	if w := get(h, http.MethodGet, "/", "Cookie", "currency=eur"); e.runs.Load() != 2 ||
		!strings.Contains(w.Body.String(), "currency=eur") {
		t.Fatalf("handler ran %d times, body %q; want one page per currency", e.runs.Load(), w.Body.String())
	}
	if w := get(
		h,
		http.MethodGet,
		"/",
		"Authorization",
		"Bearer t",
	); w.Header().
		Get("Cache-Status") !=
		"gashe; fwd=request" {
		t.Fatalf("Authorization without Vary.User was cached: %q", w.Header().Get("Cache-Status"))
	}
}

func TestVaryCustomIsPartOfTheKey(t *testing.T) {
	e := &echo{}
	h := serve(t, newCache(t), &httpcache.Options{Vary: httpcache.Vary{
		Custom: func(r *http.Request) string { return r.Header.Get("X-Tenant") },
	}}, e)
	get(h, http.MethodGet, "/", "X-Tenant", "a")
	get(h, http.MethodGet, "/", "X-Tenant", "b")
	get(h, http.MethodGet, "/", "X-Tenant", "a")
	if e.runs.Load() != 2 {
		t.Fatalf("handler ran %d times, want one per tenant", e.runs.Load())
	}
}

func TestOverloadedRequestGets503(t *testing.T) {
	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = io.WriteString(w, "page")
	})
	h := serve(t, newCache(t), &httpcache.Options{
		Policy: gashe.Policy[httpcache.Response]{TTL: time.Minute, MaxConcurrentLoads: 1},
	}, slow)
	done := make(chan struct{})
	go func() { get(h, http.MethodGet, "/a"); close(done) }() // takes the only slot
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/b", http.NoBody)
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	close(release)
	<-done
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503: rendering anyway would load what the limit holds back", w.Code)
	}
}

// A request whose server-side deadline passes while it waits for the load of another request
// still gets an answer: its client is there, only the time is up.
func TestRequestPastItsDeadlineGets503(t *testing.T) {
	release := make(chan struct{})
	slow := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		<-release
		_, _ = io.WriteString(w, "page")
	})
	h := serve(t, newCache(t), &httpcache.Options{Policy: gashe.Policy[httpcache.Response]{TTL: time.Minute}}, slow)
	done := make(chan struct{})
	go func() { get(h, http.MethodGet, "/a"); close(done) }() // renders /a
	time.Sleep(50 * time.Millisecond)

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Millisecond)
	defer cancel()
	r := httptest.NewRequestWithContext(ctx, http.MethodGet, "/a", http.NoBody) // waits for that render
	w := httptest.NewRecorder()
	h.ServeHTTP(w, r)
	close(release)
	<-done
	if w.Code != http.StatusServiceUnavailable {
		t.Fatalf("status %d, want 503 for a request out of time", w.Code)
	}
}

// deadBackend fails every call, like a Redis that is gone.
type deadBackend struct {
	gashe.Backend
}

func (deadBackend) Get(context.Context, string) ([]byte, error) { return nil, errors.New("down") }
func (deadBackend) Set(context.Context, string, []byte, time.Duration) error {
	return errors.New("down")
}

func TestFallbackServesPagesWhileTheBackendIsDown(t *testing.T) {
	spare := memory.New(memory.Options{PurgeInterval: -1})
	t.Cleanup(func() { _ = spare.Close() })
	c := gashe.New(deadBackend{Backend: newCache(t).Backend()}, gashe.Options{})
	p := &page{body: "catalog"}
	h := serve(t, c, &httpcache.Options{Policy: gashe.Policy[httpcache.Response]{
		TTL:      time.Minute,
		Fallback: &gashe.FallbackOptions{Backend: spare},
	}}, p)

	for range 3 {
		if w := get(h, http.MethodGet, "/"); w.Body.String() != "catalog" {
			t.Fatalf("body %q", w.Body.String())
		}
	}
	if p.runs.Load() != 1 {
		t.Fatalf("handler ran %d times, want once: the fallback must answer the rest", p.runs.Load())
	}
}
