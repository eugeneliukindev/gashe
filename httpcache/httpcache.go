// Package httpcache caches whole HTTP responses of a handler in a gashe.Cache. A hit does not call
// the handler at all, and every strategy of gashe.Policy applies to responses: stale-if-error
// serves the last good page while the handler fails, Stampede.LockFor renders a cold page once for all
// processes, and Tags invalidate pages together.
//
// Only GET and HEAD requests are cached, and only responses that are safe to share between
// users: see New for the rules.
package httpcache

import (
	"bytes"
	"cmp"
	"context"
	"errors"
	"fmt"
	"hash/fnv"
	"maps"
	"net/http"
	"strconv"
	"strings"
	"sync/atomic"
	"time"

	"github.com/eugeneliukindev/gashe"
)

const defaultMaxBody = 1 << 20

// Headers the middleware reads or sets.
const (
	headerAge           = "Age"
	headerAuthorization = "Authorization"
	headerCookie        = "Cookie"
	headerCacheControl  = "Cache-Control"
	headerETag          = "ETag"
	headerIfNoneMatch   = "If-None-Match"
	headerRange         = "Range"
	headerSetCookie     = "Set-Cookie"
	headerVary          = "Vary"
	headerCacheStatus   = "Cache-Status"
	headerConnection    = "Connection"
	headerContentLength = "Content-Length"
)

// cacheStatus is the Cache-Status header of RFC 9211: how this cache handled the request.
type cacheStatus string

const (
	// statusHit is a response from the cache.
	statusHit cacheStatus = "gashe; hit"
	// statusStored is a response the handler rendered for this request and the cache stored.
	statusStored cacheStatus = "gashe; fwd=uri-miss; stored"
	// statusMiss is a response the handler rendered for this request that was not stored.
	statusMiss cacheStatus = "gashe; fwd=uri-miss"
	// statusMethod is a request whose method is not cached.
	statusMethod cacheStatus = "gashe; fwd=method"
	// statusRequest is a request whose headers keep it out of the cache: Range, Authorization, Cookie.
	statusRequest cacheStatus = "gashe; fwd=request"
)

var (
	// errServer marks a 5xx response, so Stale.IfError can answer it with the last good one.
	errServer = errors.New("httpcache: server error")
	// errNotStored marks a response that must not be stored or shared with other requests.
	errNotStored = errors.New("httpcache: response not stored")
)

// serverError is a 5xx response, shared with the requests that waited for the same render when no
// header keeps it to one client.
type serverError struct {
	resp *Response
}

func (e *serverError) Error() string        { return errServer.Error() }
func (e *serverError) Is(target error) bool { return target == errServer }

// Response is a stored response. Policy.CacheIf receives it.
type Response struct {
	// Status is the status code of the response.
	Status int
	// Header holds the headers that are stored with the page.
	Header http.Header
	// Body is the body of the response.
	Body []byte
	// ETag is the ETag header of the page, set by the handler or made from a hash of Body.
	ETag string
	// Stored is when the response was stored, for the Age header.
	Stored time.Time
}

// Options configures New.
type Options struct {
	// Policy says how responses are cached, as for gashe.GetOrLoad. CacheIf decides which statuses
	// are stored; nil stores 200 only. The rules of New apply on top of it.
	Policy gashe.Policy[Response]
	// Vary says which parts of a request a page depends on, and so what its key is made of;
	// Policy.Prefix is put in front of the key. The zero value means the host, the path and the
	// query, with requests carrying Authorization or Cookie headers past the cache. HEAD reads what
	// GET stored.
	Vary Vary
	// MaxBodyBytes is the largest body that is stored; a larger one is sent but not stored.
	// 0 means 1 MB.
	MaxBodyBytes int
}

// New returns a middleware that caches the responses of a handler in c.
//
// A request goes straight to the handler if it is not GET or HEAD, has a Range header, or has an
// Authorization or Cookie header Vary does not cover. A response is stored only if CacheIf
// allows its status and it has no Set-Cookie, no Vary header beyond Options.Vary, no Cache-Control
// no-store, private or no-cache, and a body within MaxBodyBytes; otherwise it is sent as is to the
// request that made it. A 5xx response counts as a loader error, so Stale.IfError answers it with
// the last stored response; a request Stampede.MaxLoads holds back, or one whose ctx
// deadline passes while it waits for a load, gets 503.
//
// Request Cache-Control and Pragma headers are ignored, so clients cannot make the handler run on
// every request. Responses get an ETag (a hash of the body unless the handler set one), Age on a
// response from the cache, and Cache-Status (RFC 9211), with fwd=stale and fwd-status on a stale
// page Stale.IfError answered; a matching If-None-Match gets 304. It panics on a negative
// MaxBodyBytes and on Policy.Negative, which cannot apply: CacheIf stores 404 responses.
//
//nolint:gocritic // options by value, like gashe.New
func New(c *gashe.Cache, o Options) func(http.Handler) http.Handler {
	if o.MaxBodyBytes < 0 {
		panic("httpcache: negative MaxBodyBytes")
	}
	if o.Policy.Negative.IsNotFound != nil {
		// The loader returns only server errors and responses not to store, never "not found".
		panic("httpcache: Policy.Negative does not apply; store 404 responses with CacheIf")
	}
	m := &middleware{
		c:       c,
		policy:  o.Policy,
		cacheIf: o.Policy.CacheIf,
		vary:    newVary(&o.Vary),
		maxBody: cmp.Or(o.MaxBodyBytes, defaultMaxBody),
	}
	m.policy.CacheIf = nil // the loader applies it, with the rules above
	isFailure := o.Policy.Stale.IfError.IsFailure
	m.policy.Stale.IfError.IsFailure = func(err error) bool {
		return !errors.Is(err, errNotStored) && (isFailure == nil || isFailure(err))
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.serve(w, r, next) })
	}
}

type middleware struct {
	c       *gashe.Cache
	policy  gashe.Policy[Response]
	cacheIf func(Response) bool
	vary    vary
	maxBody int
}

func (m *middleware) serve(w http.ResponseWriter, r *http.Request, next http.Handler) {
	if status, ok := m.bypass(r); ok {
		w.Header().Set(headerCacheStatus, string(status))
		next.ServeHTTP(w, r)
		return
	}
	// own is the response this request rendered. A background reload may render later, so it
	// is read and written atomically.
	var own atomic.Pointer[Response]
	resp, err := gashe.GetOrLoad(r.Context(), m.c, m.vary.of(r), m.policy,
		func(ctx context.Context) (Response, error) { return m.load(ctx, r, next, &own) })
	m.answer(w, r, next, &resp, err, own.Load())
}

// load renders r for the cache and keeps the response in own: the error says why it is not stored.
func (m *middleware) load(
	ctx context.Context,
	r *http.Request,
	next http.Handler,
	own *atomic.Pointer[Response],
) (Response, error) {
	rendered, storable, shared := m.render(ctx, r, next)
	own.Store(&rendered)
	switch {
	case ctx.Err() != nil:
		return Response{}, ctx.Err() // every client left: the page may be cut short
	case rendered.Status >= http.StatusInternalServerError && shared:
		return Response{}, &serverError{resp: &rendered}
	case rendered.Status >= http.StatusInternalServerError:
		return Response{}, errServer
	case !storable:
		return Response{}, errNotStored
	}
	return rendered, nil
}

// answer writes what GetOrLoad returned: resp and err, with mine the response this request rendered.
func (m *middleware) answer(
	w http.ResponseWriter,
	r *http.Request,
	next http.Handler,
	resp *Response,
	err error,
	mine *Response,
) {
	var shared *serverError
	switch {
	case err == nil:
		write(w, r, resp, statusOf(resp, mine))
	case mine != nil:
		write(w, r, mine, statusMiss)
	case errors.As(err, &shared):
		write(w, r, shared.resp, statusMiss) // the 5xx another request rendered for this key
	case errors.Is(err, gashe.ErrOverloaded), errors.Is(err, context.DeadlineExceeded):
		// Stampede.MaxLoads is full, or the request is out of time while it waits for a
		// load: rendering anyway would load what the limit holds back, or answer too late.
		w.Header().Set(headerCacheStatus, string(statusMiss))
		http.Error(w, http.StatusText(http.StatusServiceUnavailable), http.StatusServiceUnavailable)
	case r.Context().Err() != nil:
		// The client is gone.
	default:
		// The load was shared with another request whose response cannot be shared, or the cache
		// failed in a way GetOrLoad does not work around: render for this request alone.
		w.Header().Set(headerCacheStatus, string(statusMiss))
		next.ServeHTTP(w, r)
	}
}

// statusOf tells a stored resp this request rendered, mine, from one it found in the cache, and
// from a stale one Stale.IfError answered because mine failed.
func statusOf(resp, mine *Response) cacheStatus {
	switch {
	case mine == nil:
		return statusHit
	case mine.Stored.Equal(resp.Stored):
		return statusStored
	case mine.Status >= http.StatusInternalServerError:
		return statusStale(mine.Status)
	}
	return statusHit
}

// statusStale is a stale response answered because the handler returned status.
func statusStale(status int) cacheStatus {
	return cacheStatus(fmt.Sprintf("gashe; fwd=stale; fwd-status=%d", status))
}

// bypass reports whether r must go straight to the handler, and why.
func (m *middleware) bypass(r *http.Request) (cacheStatus, bool) {
	switch {
	case r.Method != http.MethodGet && r.Method != http.MethodHead:
		return statusMethod, true
	case r.Header.Get(headerRange) != "":
		return statusRequest, true
	case !m.vary.shared(r):
		return statusRequest, true
	}
	return "", false
}

// render runs the handler as GET, also for HEAD, so the body can be stored; it reports whether
// the response may be stored, and whether it may be shared with other requests at all.
func (m *middleware) render(
	ctx context.Context,
	r *http.Request,
	next http.Handler,
) (resp Response, storable, shared bool) {
	req := r.Clone(ctx)
	req.Method = http.MethodGet
	rec := &recorder{header: http.Header{}}
	next.ServeHTTP(rec, req)
	resp = Response{
		Status: cmp.Or(rec.status, http.StatusOK),
		Header: stored(rec.header),
		Body:   rec.body.Bytes(),
		ETag:   rec.header.Get(headerETag),
		Stored: time.Now(),
	}
	if resp.ETag == "" {
		h := fnv.New64a()
		_, _ = h.Write(resp.Body)
		resp.ETag = fmt.Sprintf(`"%016x"`, h.Sum64())
	}
	shared = m.shareable(rec.header)
	return resp, shared && m.storable(&resp), shared
}

// shareable reports whether a response with headers h may go to other requests than the one that
// rendered it.
func (m *middleware) shareable(h http.Header) bool {
	return h.Get(headerSetCookie) == "" && m.vary.covers(h.Values(headerVary)) &&
		!hasDirective(h.Values(headerCacheControl), "no-store", "private", "no-cache")
}

// storable reports whether a shareable response is stored.
func (m *middleware) storable(resp *Response) bool {
	switch {
	case len(resp.Body) > m.maxBody:
		return false
	case m.cacheIf != nil:
		return m.cacheIf(*resp)
	}
	return resp.Status == http.StatusOK
}

// write sends resp with its Cache-Status.
func write(w http.ResponseWriter, r *http.Request, resp *Response, status cacheStatus) {
	h := w.Header()
	maps.Copy(h, resp.Header)
	h.Set(headerETag, resp.ETag)
	h.Set(headerCacheStatus, string(status))
	if status != statusStored && status != statusMiss { // from the cache, fresh or stale
		h.Set(headerAge, strconv.Itoa(int(time.Since(resp.Stored).Seconds())))
	}
	if resp.Status == http.StatusOK && etagMatches(r.Header.Get(headerIfNoneMatch), resp.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	h.Set(headerContentLength, strconv.Itoa(len(resp.Body))) // HEAD has the headers of GET
	w.WriteHeader(resp.Status)
	if r.Method != http.MethodHead {
		_, _ = w.Write(resp.Body) // a failed write means the client is gone
	}
}

// stored returns the headers worth storing: without hop-by-hop ones and those the server sets.
func stored(h http.Header) http.Header {
	out := h.Clone()
	for _, v := range h.Values(headerConnection) { // fields it names are hop-by-hop too (RFC 9110 7.6.1)
		for _, name := range strings.Split(v, ",") {
			out.Del(strings.TrimSpace(name))
		}
	}
	for _, k := range []string{
		"Connection", "Keep-Alive", "Proxy-Authenticate", "Proxy-Authorization", "Te", "Trailer",
		"Transfer-Encoding", "Upgrade", "Date", "Content-Length", "Etag",
	} {
		out.Del(k)
	}
	return out
}

// hasDirective reports whether a Cache-Control header holds one of names, with or without a value.
func hasDirective(values []string, names ...string) bool {
	for _, v := range values {
		for _, d := range strings.Split(v, ",") {
			name, _, _ := strings.Cut(strings.TrimSpace(d), "=")
			for _, n := range names {
				if strings.EqualFold(name, n) {
					return true
				}
			}
		}
	}
	return false
}

// etagMatches compares If-None-Match with etag the weak way RFC 9110 asks for: W/ is ignored.
func etagMatches(header, etag string) bool {
	if header == "" {
		return false
	}
	etag = strings.TrimPrefix(etag, "W/")
	for _, t := range strings.Split(header, ",") {
		t = strings.TrimSpace(t)
		if t == "*" || strings.TrimPrefix(t, "W/") == etag {
			return true
		}
	}
	return false
}

// recorder keeps what the handler writes.
type recorder struct {
	header http.Header
	status int
	body   bytes.Buffer
}

func (rec *recorder) Header() http.Header { return rec.header }

func (rec *recorder) WriteHeader(status int) {
	if rec.status == 0 {
		rec.status = status
	}
}

func (rec *recorder) Write(p []byte) (int, error) {
	rec.WriteHeader(http.StatusOK)
	return rec.body.Write(p)
}
