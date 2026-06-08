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

// Instrument returns middleware that records timing, status, and response size
// for each request and reports them to obs. It is dependency-free; wire obs to
// your telemetry backend of choice.
//
//	router.Use(plugins.Instrument(plugins.ObserverFunc(func(i plugins.RequestInfo) {
//		metrics.RequestDuration.WithLabelValues(i.Method, i.Route, strconv.Itoa(i.Status)).Observe(i.Duration.Seconds())
//	})))
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

			route := server.RoutePattern(r)
			if route == "" {
				route = r.URL.Path
			}
			obs.ObserveRequest(RequestInfo{
				Method:   r.Method,
				Route:    route,
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

// meteredWriter records the status code and number of body bytes written while
// transparently forwarding the optional ResponseWriter interfaces.
type meteredWriter struct {
	http.ResponseWriter
	status      int
	bytes       int64
	wroteHeader bool
}

func (m *meteredWriter) WriteHeader(status int) {
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
