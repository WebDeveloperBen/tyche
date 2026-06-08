package plugins

import (
	"bufio"
	"io"
	"net"
	"net/http"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

// RequestInfo is the set of metrics captured for a single handled request. It
// is passed to a [RequestObserver] after the handler completes.
type RequestInfo struct {
	// Method is the HTTP method.
	Method string
	// Route is the matched route template (e.g. "/users/:id"), suitable as a
	// low-cardinality metric label. It falls back to the request path when no
	// template is available.
	Route string
	// Path is the concrete request path.
	Path string
	// Status is the HTTP status code written by the handler (defaults to 200
	// when the handler writes a body without an explicit status).
	Status int
	// Bytes is the number of response body bytes written.
	Bytes int64
	// Duration is the wall-clock time spent in the handler chain.
	Duration time.Duration
	// Err is the error returned by the handler, if any.
	Err error
}

// RequestObserver receives a [RequestInfo] for every handled request. It is the
// integration seam for tracing and metrics backends: implement it to bridge to
// OpenTelemetry, Prometheus, StatsD, or any sink. Implementations must be safe
// for concurrent use and should not block.
type RequestObserver interface {
	ObserveRequest(info RequestInfo)
}

// ObserverFunc adapts a plain function to a [RequestObserver].
type ObserverFunc func(RequestInfo)

// ObserveRequest calls f(info).
func (f ObserverFunc) ObserveRequest(info RequestInfo) { f(info) }

// Instrument returns HandlerFunc middleware that records timing and the
// handler's returned error for each request and reports them to obs. It is
// dependency-free; wire obs to your telemetry backend of choice.
//
//	router.Use(plugins.Instrument(plugins.ObserverFunc(func(i plugins.RequestInfo) {
//		metrics.RequestDuration.WithLabelValues(i.Method, i.Route, strconv.Itoa(i.Status)).Observe(i.Duration.Seconds())
//	})))
//
// IMPORTANT: because this runs inside the router's error-handling boundary, the
// observed Status/Bytes reflect only what the handler itself wrote. When a
// handler returns an error, the router renders the response afterwards, so
// Status will be the handler's default (200) rather than the final status —
// use RequestInfo.Err to classify those. For accurate final status and bytes
// (including error and 404/405 responses), use [InstrumentHTTP] instead, which
// wraps the entire router.
//
// The wrapper preserves http.Flusher, http.Hijacker, io.ReaderFrom, and
// http.Pusher, so it composes safely with streaming (Server-Sent Events) and
// WebSocket upgrade handlers.
func Instrument(obs RequestObserver) server.Middleware {
	if obs == nil {
		return func(next server.HandlerFunc) server.HandlerFunc { return next }
	}
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			start := time.Now()
			mw := &meteredWriter{ResponseWriter: w, status: http.StatusOK}
			err := next(mw, r)

			obs.ObserveRequest(RequestInfo{
				Method:   r.Method,
				Route:    routeLabel(r),
				Path:     r.URL.Path,
				Status:   mw.status,
				Bytes:    mw.bytes,
				Duration: time.Since(start),
				Err:      err,
			})
			return err
		}
	}
}

// InstrumentHTTP returns http.Handler middleware (for [server.Router.UseHTTP])
// that records timing, the final response status, and bytes written for every
// request — including responses produced by the router's error handler and the
// not-found / method-not-allowed handlers. It wraps the whole router, so unlike
// [Instrument] the Status is always the true status sent to the client.
//
// It cannot observe the handler's returned Go error (the router has already
// converted it to a response by this layer), so RequestInfo.Err is always nil;
// classify failures by Status. This is the recommended seam for metrics.
//
//	router.UseHTTP(plugins.InstrumentHTTP(obs))
func InstrumentHTTP(obs RequestObserver) server.ServeHTTPMiddleware {
	if obs == nil {
		return func(next http.Handler) http.Handler { return next }
	}
	return func(next http.Handler) http.Handler {
		return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			start := time.Now()
			mw := &meteredWriter{ResponseWriter: w, status: http.StatusOK}
			next.ServeHTTP(mw, r)

			obs.ObserveRequest(RequestInfo{
				Method:   r.Method,
				Route:    routeLabel(r),
				Path:     r.URL.Path,
				Status:   mw.status,
				Bytes:    mw.bytes,
				Duration: time.Since(start),
			})
		})
	}
}

func routeLabel(r *http.Request) string {
	if route := server.RoutePattern(r); route != "" {
		return route
	}
	return r.URL.Path
}

// InstrumentPlugin is the [server] plugin form of [Instrument], usable with a
// plugin registry.
type InstrumentPlugin struct {
	Observer RequestObserver
}

// Register installs the instrumentation middleware on the router.
func (p InstrumentPlugin) Register(r *server.Router) error {
	r.Use(Instrument(p.Observer))
	return nil
}

// InstrumentHTTPPlugin is the [server] plugin form of [InstrumentHTTP].
type InstrumentHTTPPlugin struct {
	Observer RequestObserver
}

// Register installs the http.Handler-level instrumentation on the router.
func (p InstrumentHTTPPlugin) Register(r *server.Router) error {
	r.UseHTTP(InstrumentHTTP(p.Observer))
	return nil
}

// meteredWriter records the status code and number of body bytes written while
// transparently forwarding the optional ResponseWriter interfaces.
type meteredWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (m *meteredWriter) WriteHeader(status int) {
	// 1xx are informational and may precede the final header (e.g. 100 Continue,
	// 103 Early Hints). Don't latch them as the response status or mark the
	// response as written — otherwise metrics would report the 1xx and, under
	// InstrumentHTTP, the router would suppress a later error response.
	if status >= 100 && status < 200 {
		m.ResponseWriter.WriteHeader(status)
		return
	}
	if !m.wroteHeader {
		m.status = status
		m.wroteHeader = true
	}
	m.ResponseWriter.WriteHeader(status)
}

func (m *meteredWriter) Write(b []byte) (int, error) {
	if !m.wroteHeader {
		m.wroteHeader = true
	}
	n, err := m.ResponseWriter.Write(b)
	m.bytes += int64(n)
	return n, err
}

// Written reports whether a response has begun, satisfying the router's
// writtenChecker so error rendering does not double-write when this wraps the
// whole router via InstrumentHTTP.
func (m *meteredWriter) Written() bool { return m.wroteHeader }

func (m *meteredWriter) Flush() {
	if f, ok := m.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (m *meteredWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := m.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (m *meteredWriter) ReadFrom(src io.Reader) (int64, error) {
	if rf, ok := m.ResponseWriter.(io.ReaderFrom); ok {
		if !m.wroteHeader {
			m.wroteHeader = true
		}
		n, err := rf.ReadFrom(src)
		m.bytes += n
		return n, err
	}
	n, err := io.Copy(m.ResponseWriter, src)
	m.bytes += n
	return n, err
}

func (m *meteredWriter) Push(target string, opts *http.PushOptions) error {
	if p, ok := m.ResponseWriter.(http.Pusher); ok {
		return p.Push(target, opts)
	}
	return http.ErrNotSupported
}

// Unwrap exposes the underlying writer to http.ResponseController.
func (m *meteredWriter) Unwrap() http.ResponseWriter {
	return m.ResponseWriter
}
