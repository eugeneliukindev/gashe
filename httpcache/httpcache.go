// Package httpcache caches whole HTTP responses of a handler in a gashe.Cache. A hit does not call
// the handler at all, and every strategy of gashe.Policy applies to responses: stale-if-error
// serves the last good page while the handler fails, LockOnMiss renders a cold page once for all
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
	headerCacheControl  = "Cache-Control"
	headerETag          = "ETag"
	headerIfNoneMatch   = "If-None-Match"
	headerRange         = "Range"
	headerSetCookie     = "Set-Cookie"
	headerVary          = "Vary"
	headerCacheStatus   = "Cache-Status"
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
	// statusRequest is a request whose headers keep it out of the cache: Range, Authorization.
	statusRequest cacheStatus = "gashe; fwd=request"
)

var (
	// errServer marks a 5xx response, so StaleIfError can answer it with the last good one.
	errServer = errors.New("httpcache: server error")
	// errNotStored marks a response that must not be stored or shared with other requests.
	errNotStored = errors.New("httpcache: response not stored")
)

// Response is a stored response. Policy.CacheIf receives it.
type Response struct {
	Status int
	Header http.Header
	Body   []byte
	ETag   string
	Stored time.Time
}

// Options configures New.
type Options struct {
	// Policy says how responses are cached, as for gashe.GetOrLoad. CacheIf decides which statuses
	// are stored; nil stores 200 only. The rules of New apply on top of it.
	Policy gashe.Policy[Response]
	// Key returns the key of a request; Policy.Prefix is put in front of it. It must not depend on
	// the method: HEAD reads what GET stored. nil means host, path and query, and requests with an
	// Authorization header are not cached. With your own Key, they are: include the user in it.
	Key func(r *http.Request) string
	// MaxBodyBytes is the largest body that is stored; a larger one is sent but not stored.
	// 0 means 1 MB.
	MaxBodyBytes int
}

// New returns a middleware that caches the responses of a handler in c.
//
// A request goes straight to the handler if it is not GET or HEAD, has a Range header, or, with
// the default Key, has an Authorization header. A response is stored only if CacheIf allows its
// status and it has no Set-Cookie, no Vary, no Cache-Control no-store, private or no-cache, and a
// body within MaxBodyBytes; otherwise it is sent as is to the request that made it. A 5xx response
// counts as a loader error, so StaleIfError answers it with the last stored response.
//
// Request Cache-Control and Pragma headers are ignored, so clients cannot make the handler run on
// every request. Responses get an ETag (a hash of the body unless the handler set one), Age on a
// hit, and Cache-Status (RFC 9211); a matching If-None-Match gets 304.
//
//nolint:gocritic // options by value, like gashe.New
func New(c *gashe.Cache, o Options) func(http.Handler) http.Handler {
	if o.MaxBodyBytes < 0 {
		panic("httpcache: negative MaxBodyBytes")
	}
	m := &middleware{
		c:       c,
		policy:  o.Policy,
		cacheIf: o.Policy.CacheIf,
		key:     o.Key,
		maxBody: cmp.Or(o.MaxBodyBytes, defaultMaxBody),
	}
	m.policy.CacheIf = nil // the loader applies it, with the rules above
	staleIf := o.Policy.StaleIf
	m.policy.StaleIf = func(err error) bool {
		return !errors.Is(err, errNotStored) && (staleIf == nil || staleIf(err))
	}
	if negIf := o.Policy.Negative.If; negIf != nil {
		m.policy.Negative.If = func(err error) bool {
			return !errors.Is(err, errNotStored) && !errors.Is(err, errServer) && negIf(err)
		}
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { m.serve(w, r, next) })
	}
}

type middleware struct {
	c       *gashe.Cache
	policy  gashe.Policy[Response]
	cacheIf func(Response) bool
	key     func(r *http.Request) string
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
	resp, err := gashe.GetOrLoad(r.Context(), m.c, m.requestKey(r), m.policy,
		func(ctx context.Context) (Response, error) {
			rendered, storable := m.render(ctx, r, next)
			own.Store(&rendered)
			switch {
			case rendered.Status >= http.StatusInternalServerError:
				return Response{}, errServer
			case !storable:
				return Response{}, errNotStored
			}
			return rendered, nil
		})
	mine := own.Load()
	switch {
	case err == nil:
		write(w, r, &resp, statusOf(&resp, mine))
	case mine != nil:
		write(w, r, mine, statusMiss)
	case r.Context().Err() != nil:
		// The client is gone.
	default:
		// The load was shared with another request whose response cannot be shared, or the cache
		// failed in a way GetOrLoad does not work around: render for this request alone.
		w.Header().Set(headerCacheStatus, string(statusMiss))
		next.ServeHTTP(w, r)
	}
}

// statusOf tells a stored resp this request rendered, mine, from one it found in the cache.
func statusOf(resp, mine *Response) cacheStatus {
	if mine != nil && mine.Stored.Equal(resp.Stored) {
		return statusStored
	}
	return statusHit
}

// bypass reports whether r must go straight to the handler, and why.
func (m *middleware) bypass(r *http.Request) (cacheStatus, bool) {
	switch {
	case r.Method != http.MethodGet && r.Method != http.MethodHead:
		return statusMethod, true
	case r.Header.Get(headerRange) != "":
		return statusRequest, true
	case m.key == nil && r.Header.Get(headerAuthorization) != "":
		return statusRequest, true
	}
	return "", false
}

func (m *middleware) requestKey(r *http.Request) string {
	if m.key != nil {
		return m.key(r)
	}
	return r.Host + r.URL.RequestURI()
}

// render runs the handler as GET, also for HEAD, so the body can be stored; it reports whether
// the response may be stored.
func (m *middleware) render(ctx context.Context, r *http.Request, next http.Handler) (Response, bool) {
	req := r.Clone(ctx)
	req.Method = http.MethodGet
	rec := &recorder{header: http.Header{}}
	next.ServeHTTP(rec, req)
	resp := Response{
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
	ok := m.storable(&resp, rec.header)
	return resp, ok
}

func (m *middleware) storable(resp *Response, h http.Header) bool {
	switch {
	case len(resp.Body) > m.maxBody, h.Get(headerSetCookie) != "", h.Get(headerVary) != "":
		return false
	case hasDirective(h.Values(headerCacheControl), "no-store", "private", "no-cache"):
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
	if status == statusHit {
		h.Set(headerAge, strconv.Itoa(int(time.Since(resp.Stored).Seconds())))
	}
	if resp.Status == http.StatusOK && etagMatches(r.Header.Get(headerIfNoneMatch), resp.ETag) {
		w.WriteHeader(http.StatusNotModified)
		return
	}
	w.WriteHeader(resp.Status)
	if r.Method != http.MethodHead {
		//nolint:gosec // the handler's own output, replayed; a failed write means the client is gone
		_, _ = w.Write(resp.Body)
	}
}

// stored returns the headers worth storing: without hop-by-hop ones and those the server sets.
func stored(h http.Header) http.Header {
	out := h.Clone()
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
