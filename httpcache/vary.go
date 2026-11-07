package httpcache

import (
	"net/http"
	"net/url"
	"slices"
	"strings"
)

// Vary says which parts of a request a page depends on, as the Vary header of RFC 9110 does for
// headers, and so what the key of a request is made of. The host and the path are always part of
// the key; of the rest, only what Vary names is, so two requests that differ only in other
// headers, cookies or query parameters get the same page. Every part of a request the handler
// builds the page from must be in it, or one request's page is served to another.
type Vary struct {
	// Headers are request headers whose values are part of the key: Accept-Language for pages in
	// the language of the request, X-Forwarded-Host behind a proxy that rewrites Host. A response
	// whose Vary names only these headers is stored; any other Vary keeps it out.
	// nil means none.
	Headers []string
	// Query lists the query parameters that are part of the key; the others, such as utm_source or
	// a random one, do not make new entries. nil means the whole query as sent; an empty list means
	// none.
	Query []string
	// Cookies lists the cookies the page depends on, such as a currency; their values are part of
	// the key, and requests with other cookies are cached too. nil means a request with a Cookie
	// header goes past the cache, unless User is set: its page may belong to who sent it.
	Cookies []string
	// User returns who the page is for, such as the user id of the session; pages are then stored
	// per user, and requests with Cookie or Authorization headers are cached. nil means pages are
	// shared, and requests with an Authorization header go past the cache.
	User func(r *http.Request) string
	// Custom returns anything else the page depends on; it is added at the end of the key.
	// nil means none.
	Custom func(r *http.Request) string
}

// vary is a Vary with its header names canonical, ready for every request.
type vary struct {
	Vary

	varies []string // the canonical headers a Vary may name: Headers, and Cookie or Authorization
}

func newVary(k *Vary) vary {
	out := vary{Vary: *k}
	out.Headers = make([]string, len(k.Headers))
	for i, h := range k.Headers {
		out.Headers[i] = http.CanonicalHeaderKey(h)
	}
	out.Query = slices.Clone(k.Query)
	slices.Sort(out.Query)
	out.varies = slices.Clone(out.Headers)
	if k.Cookies != nil || k.User != nil {
		out.varies = append(out.varies, headerCookie)
	}
	if k.User != nil {
		out.varies = append(out.varies, headerAuthorization)
	}
	return out
}

// shared reports whether r may be answered from a page stored for others: its Authorization or
// Cookie header could make the page its own unless the key covers them.
func (k *vary) shared(r *http.Request) bool {
	if k.User != nil {
		return true
	}
	return r.Header.Get(headerAuthorization) == "" && (k.Cookies != nil || r.Header.Get(headerCookie) == "")
}

// of returns the key of r.
func (k *vary) of(r *http.Request) string {
	var b strings.Builder
	b.WriteString(r.Host + r.URL.EscapedPath())
	if q := k.query(r); q != "" {
		b.WriteString("?" + q)
	}
	for _, h := range k.Headers {
		b.WriteString(" h:" + h + "=" + url.QueryEscape(r.Header.Get(h)))
	}
	for _, name := range k.Cookies {
		value := ""
		if c, err := r.Cookie(name); err == nil {
			value = c.Value
		}
		b.WriteString(" c:" + name + "=" + url.QueryEscape(value))
	}
	if k.User != nil {
		b.WriteString(" u=" + url.QueryEscape(k.User(r)))
	}
	if k.Custom != nil {
		b.WriteString(" x=" + url.QueryEscape(k.Custom(r)))
	}
	return b.String()
}

// query returns the part of the query of r that is part of the key.
func (k *vary) query(r *http.Request) string {
	if k.Query == nil {
		return r.URL.RawQuery
	}
	q := r.URL.Query()
	kept := url.Values{}
	for _, name := range k.Query {
		if vs, ok := q[name]; ok {
			kept[name] = vs
		}
	}
	return kept.Encode() // Encode sorts, so the order of a request does not matter
}

// covers reports whether every header a Vary header of a response names is part of the key.
func (k *vary) covers(names []string) bool {
	for _, v := range names {
		for _, name := range strings.Split(v, ",") {
			name = http.CanonicalHeaderKey(strings.TrimSpace(name))
			if name != "" && !slices.Contains(k.varies, name) {
				return false // "*" too: it is never part of the key
			}
		}
	}
	return true
}
