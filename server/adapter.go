package server

import (
	"net/http"
	"strings"
	"sync"
)

// Adapter is the bring-your-own-router seam.
//
// An adapter owns exactly one thing: path matching. It maps a (method, path)
// pair to a handler and dispatches incoming requests to the right one,
// including 404/405 behaviour. Everything tyche is actually good at —
// generated codecs, validation, the {"data":…} envelope, problem+json errors,
// middleware composition, and OpenAPI generation — is layered on top by [API]
// and is independent of which adapter is used.
//
// Paths are handed to Handle in tyche's native template form (":name" for a
// path parameter, "*name" for a trailing wildcard). Each adapter translates
// that to its router's own syntax. Adapters MUST ensure matched path
// parameters are readable through [Param] (i.e. via (*http.Request).PathValue)
// so the existing codec and reflection binders work unchanged.
type Adapter interface {
	// Handle registers h to serve method+path.
	Handle(method, path string, h http.Handler)
	// ServeHTTP dispatches a request to the matching handler, falling back to
	// the handlers set via SetFallback when nothing matches.
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	// SetFallback installs the handler for unmatched paths (notFound) and for a
	// path that matches a route registered under a different method
	// (methodNotAllowed). Adapters that cannot distinguish the two may route
	// both to notFound.
	SetFallback(notFound, methodNotAllowed http.Handler)
}

// ---- net/http.ServeMux adapter (the zero-dependency default) ----

// ServeMuxAdapter routes with the standard library's http.ServeMux (Go 1.22+),
// which already supports method matching, "{name}" path parameters, and
// "{name...}" trailing wildcards — and populates (*http.Request).PathValue,
// exactly what tyche's binders read. That makes it a drop-in with no changes to
// the codec layer.
type ServeMuxAdapter struct {
	mux              *http.ServeMux
	notFound         http.Handler
	methodNotAllowed http.Handler
}

func NewServeMuxAdapter() *ServeMuxAdapter {
	return &ServeMuxAdapter{mux: http.NewServeMux()}
}

func (a *ServeMuxAdapter) Handle(method, path string, h http.Handler) {
	a.mux.Handle(method+" "+toStdlibPattern(path), matchWrap(h, trailingWildcardName(path)))
}

func (a *ServeMuxAdapter) SetFallback(notFound, methodNotAllowed http.Handler) {
	a.notFound = notFound
	a.methodNotAllowed = methodNotAllowed
}

var fallbackInterceptorPool = sync.Pool{
	New: func() any { return &fallbackInterceptor{} },
}

func (a *ServeMuxAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if a.notFound == nil && a.methodNotAllowed == nil {
		a.mux.ServeHTTP(w, r)
		return
	}
	// Serve through ServeMux so it populates req.PathValue for matched routes.
	// The interceptor forwards a matched handler's response verbatim; it only
	// suppresses ServeMux's own built-in 404/405 (matched == false), which we
	// re-render through the configured problem+json fallback. Any other
	// unmatched response ServeMux produces — notably 3xx redirects for
	// non-canonical paths (trailing slash, "//", "/.") — is passed through
	// untouched. The interceptor is pooled so the common path allocates nothing.
	fb := fallbackInterceptorPool.Get().(*fallbackInterceptor)
	fb.ResponseWriter = w
	fb.matched = false
	fb.suppressed = false
	fb.status = 0

	a.mux.ServeHTTP(fb, r)

	matched, suppressed, status := fb.matched, fb.suppressed, fb.status
	fb.ResponseWriter = nil
	fallbackInterceptorPool.Put(fb)

	// Matched handler responded, or ServeMux wrote a real response (e.g. a
	// redirect) that the interceptor passed through — nothing more to do.
	if matched || !suppressed {
		return
	}
	if status == http.StatusMethodNotAllowed && a.methodNotAllowed != nil {
		a.methodNotAllowed.ServeHTTP(w, r)
		return
	}
	if a.notFound != nil {
		a.notFound.ServeHTTP(w, r)
		return
	}
	// No custom fallback configured for this class (only reachable if a
	// fallback was set to nil, since NewAPI installs defaults for both).
	w.WriteHeader(status)
}

// matchWrap flags the interceptor the moment a real route handler runs (so
// ServeHTTP can tell a matched response apart from ServeMux's built-in 404/405
// fallback) and, for a trailing wildcard route, mirrors ServeMux's capture onto
// PathValue("*") so tyche's Wildcard / Param(r, "*") resolve it regardless of
// the capture name ServeMux uses.
func matchWrap(h http.Handler, wildcardName string) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if fb, ok := w.(*fallbackInterceptor); ok {
			fb.matched = true
		}
		if wildcardName != "" {
			r.SetPathValue("*", r.PathValue(wildcardName))
		}
		h.ServeHTTP(w, r)
	})
}

// fallbackInterceptor wraps the response writer during ServeMux dispatch. Once
// a real handler runs (matched == true) it is a transparent pass-through. When
// no handler matches, it suppresses ServeMux's built-in 404/405 (so the adapter
// can substitute tyche's problem+json response) but passes any other response —
// e.g. a 3xx redirect for a non-canonical path — straight through. It forwards
// Flush so streaming (SSE) handlers keep working.
type fallbackInterceptor struct {
	http.ResponseWriter
	matched    bool
	suppressed bool
	status     int
}

func (f *fallbackInterceptor) WriteHeader(code int) {
	// Only ServeMux's own 404/405 (no matched handler) is captured for
	// re-rendering; everything else — matched handlers and unmatched redirects
	// — writes through unchanged.
	if !f.matched && (code == http.StatusNotFound || code == http.StatusMethodNotAllowed) {
		f.status = code
		f.suppressed = true
		return
	}
	f.ResponseWriter.WriteHeader(code)
}

func (f *fallbackInterceptor) Write(p []byte) (int, error) {
	if f.suppressed {
		return len(p), nil // swallow the built-in 404/405 body
	}
	return f.ResponseWriter.Write(p)
}

func (f *fallbackInterceptor) Flush() {
	if fl, ok := f.ResponseWriter.(http.Flusher); ok {
		fl.Flush()
	}
}

// Written reports whether the underlying writer has started the response, so
// the error handler can avoid overwriting a partial response written by a
// matched handler before it returned an error.
func (f *fallbackInterceptor) Written() bool {
	if wc, ok := f.ResponseWriter.(writtenChecker); ok {
		return wc.Written()
	}
	return f.status != 0
}

// stdlibWildcardName is the ServeMux capture name used for an unnamed trailing
// wildcard ("*"). It must be a valid Go identifier (ServeMux requires one), so
// it cannot be "*"; matchWrap mirrors it back onto PathValue("*").
const stdlibWildcardName = "rest"

// toStdlibPattern converts "/things/:id/*rest" to "/things/{id}/{rest...}".
func toStdlibPattern(path string) string {
	if !strings.ContainsAny(path, ":*") {
		return path
	}
	var b strings.Builder
	for _, part := range SplitRouteFast(path) {
		b.WriteByte('/')
		switch {
		case len(part) > 0 && part[0] == ':':
			b.WriteByte('{')
			b.WriteString(part[1:])
			b.WriteByte('}')
		case len(part) > 0 && part[0] == '*':
			name := part[1:]
			if name == "" {
				name = stdlibWildcardName
			}
			b.WriteByte('{')
			b.WriteString(name)
			b.WriteString("...}")
		default:
			b.WriteString(part)
		}
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

// trailingWildcardName returns the ServeMux capture name of a trailing "*"
// wildcard in path, or "" if the path has no trailing wildcard.
func trailingWildcardName(path string) string {
	parts := SplitRouteFast(path)
	if len(parts) == 0 {
		return ""
	}
	last := parts[len(parts)-1]
	if len(last) == 0 || last[0] != '*' {
		return ""
	}
	if len(last) == 1 {
		return stdlibWildcardName
	}
	return last[1:]
}

// The core ships only the Adapter contract and the stdlib ServeMuxAdapter.
// Concrete adapters for third-party routers (chi, gin, fiber, …) are user-land:
// implementing the interface is ~40 lines (see the reference chi adapter in
// adapter_spike_test.go), and keeping them out of core means importing the
// server package never pulls a third-party router into the build graph.
