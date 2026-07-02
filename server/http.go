package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"strconv"
	"strings"
	"sync"

	"github.com/webdeveloperben/tyche/server/validation"
)

var validationErrorBufPool = sync.Pool{
	New: func() any {
		buf := make([]byte, 0, 512)
		return &buf
	},
}

var trackedResponseWriterPool = sync.Pool{
	New: func() any {
		return &trackedResponseWriter{}
	},
}

type Middleware func(next HandlerFunc) HandlerFunc

type HandlerFunc func(http.ResponseWriter, *http.Request) error

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// When a HandlerFunc is used directly as an http.Handler there is no error
	// sink; the error is intentionally discarded. Use it through the API to
	// route errors via the configured ErrorHandler.
	_ = f(w, r)
}

type HTTPError struct {
	Message    string
	StatusCode int
	Silent     bool
}

func (e HTTPError) Error() string {
	return e.Message
}

func NewHTTPError(statusCode int, message string) HTTPError {
	return HTTPError{StatusCode: statusCode, Message: message}
}

func SilentHTTPError(statusCode int, message string) HTTPError {
	return HTTPError{StatusCode: statusCode, Message: message, Silent: true}
}

type ServeHTTPMiddleware func(next http.Handler) http.Handler

type OpenAPIInfo struct {
	Title       string
	Description string
	Version     string
}

// APIConfig configures an [API]. The zero value is valid; NewAPI fills in
// sensible defaults (title "API", version "1.0.0", 10 MiB body limit).
type APIConfig struct {
	ErrorHandler        ErrorHandler
	OpenAPI             OpenAPIInfo
	MaxRequestBodyBytes int64
}

// ErrorHandler converts an error returned by a HandlerFunc (or produced by the
// API, e.g. path traversal) into an HTTP response. Implementations should
// respect any response already written; see DefaultErrorHandler.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

func mergeAPIConfig(base, override APIConfig) APIConfig {
	if override.OpenAPI.Title != "" {
		base.OpenAPI.Title = override.OpenAPI.Title
	}
	if override.OpenAPI.Description != "" {
		base.OpenAPI.Description = override.OpenAPI.Description
	}
	if override.OpenAPI.Version != "" {
		base.OpenAPI.Version = override.OpenAPI.Version
	}
	if override.MaxRequestBodyBytes > 0 {
		base.MaxRequestBodyBytes = override.MaxRequestBodyBytes
	}
	if override.ErrorHandler != nil {
		base.ErrorHandler = override.ErrorHandler
	}
	return base
}

func HasPathTraversal(path string) bool {
	for i := 0; i < len(path); i++ {
		if path[i] == '.' && i+1 < len(path) && path[i+1] == '.' {
			if i == 0 || path[i-1] == '/' {
				if i+2 == len(path) || path[i+2] == '/' {
					return true
				}
			}
		}
	}
	return false
}

// SplitRouteFast splits a route into its "/"-separated segments without
// allocating substrings for the separators. It panics on a route that does not
// start with "/". A bare "/" yields no segments.
func SplitRouteFast(route string) []string {
	if route == "/" {
		return nil
	}
	n := len(route)
	if n < 2 || route[0] != '/' {
		panic("invalid route: " + route)
	}

	segCount := 1
	for i := 1; i < n; i++ {
		if route[i] == '/' {
			segCount++
		}
	}

	parts := make([]string, 0, segCount)
	start := 1
	for i := 1; i < n; i++ {
		if route[i] == '/' {
			parts = append(parts, route[start:i])
			start = i + 1
		}
	}
	parts = append(parts, route[start:])
	return parts
}

func validateRoutePattern(pattern string) error {
	parts := SplitRouteFast(pattern)
	for i, part := range parts {
		if len(part) == 0 || part[0] != '*' {
			continue
		}
		if i != len(parts)-1 {
			return fmt.Errorf("wildcard must be the final path segment: %s", pattern)
		}
	}
	return nil
}

// wildcardParamName is the parameter name used for an unnamed trailing wildcard
// ("*"). It is shared by the OpenAPI path renderer and the ServeMux adapter so
// the two never disagree on what to call it.
const wildcardParamName = "wildcard"

// rewriteSegments rebuilds a route path, replacing each ":name" and "*name"
// segment via param and wildcard (which receive the segment name — "" for an
// unnamed wildcard) and copying literal segments unchanged. The caller is
// responsible for the leading-slash guard; a path with no params should skip
// this. It is the single place tyche's route-template syntax is parsed.
func rewriteSegments(path string, param, wildcard func(name string) string) string {
	var b strings.Builder
	for _, part := range SplitRouteFast(path) {
		b.WriteByte('/')
		switch {
		case len(part) > 0 && part[0] == ':':
			b.WriteString(param(part[1:]))
		case len(part) > 0 && part[0] == '*':
			b.WriteString(wildcard(part[1:]))
		default:
			b.WriteString(part)
		}
	}
	if b.Len() == 0 {
		return "/"
	}
	return b.String()
}

func joinPath(prefix, pattern string) string {
	if prefix == "" {
		return pattern
	}
	if pattern == "/" {
		return prefix
	}
	return prefix + pattern
}

var mountMethods = []string{
	http.MethodGet,
	http.MethodHead,
	http.MethodPost,
	http.MethodPut,
	http.MethodPatch,
	http.MethodDelete,
	http.MethodOptions,
}

func Param(req *http.Request, name string) string {
	if req == nil {
		return ""
	}
	return req.PathValue(name)
}

func Wildcard(req *http.Request) string {
	return Param(req, "*")
}

// RoutePattern returns the route template that matched the request (for
// example "/users/:id"), or "" if no tyche route matched. It is suitable as a
// low-cardinality label for logs and metrics, unlike the concrete request path.
func RoutePattern(req *http.Request) string {
	if req == nil {
		return ""
	}
	return req.Pattern
}

// DefaultErrorHandler is the [ErrorHandler] used when an API is not given a
// custom one. It maps [HTTPError] and validation errors to RFC 9457
// problem+json responses, treats unknown errors as 500, and never overwrites a
// response that a handler already started writing.
func DefaultErrorHandler(w http.ResponseWriter, _ *http.Request, err error) {
	handleHTTPError(w, err)
}

func defaultNotFoundHandler(w http.ResponseWriter, _ *http.Request) {
	writeErrorJSON(w, http.StatusNotFound, http.StatusText(http.StatusNotFound))
}

func defaultMethodNotAllowedHandler(w http.ResponseWriter, _ *http.Request) {
	writeErrorJSON(w, http.StatusMethodNotAllowed, http.StatusText(http.StatusMethodNotAllowed))
}

func handleHTTPError(w http.ResponseWriter, err error) {
	written := false
	if wc, ok := w.(writtenChecker); ok {
		written = wc.Written()
	}

	var validationErr *validation.Error
	if errors.As(err, &validationErr) {
		if written {
			logSuppressedError(err)
			return
		}
		writeValidationProblemJSON(w, http.StatusBadRequest, validationErr)
		return
	}
	if httpErr, ok := err.(HTTPError); ok {
		if httpErr.Silent {
			slog.Debug(
				"server: silent HTTP error suppressed",
				"status", httpErr.StatusCode,
				"message", httpErr.Message,
			)
			return
		}
		if written {
			logSuppressedError(err)
			return
		}
		message := httpErr.Message
		if message == "" {
			message = http.StatusText(httpErr.StatusCode)
		}
		writeErrorJSON(w, httpErr.StatusCode, message)
		return
	}
	var httpErr HTTPError
	if errors.As(err, &httpErr) {
		handleHTTPError(w, httpErr)
		return
	}
	if written {
		logSuppressedError(err)
		return
	}
	writeErrorJSON(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
}

// logSuppressedError records an error that cannot be rendered because the
// response has already started (e.g. a streaming handler that fails after its
// first event). The status cannot change at that point, so the error is logged
// rather than silently dropped.
func logSuppressedError(err error) {
	slog.Warn("server: handler error after response started; suppressed", "error", err)
}

type problemDetails struct {
	Type   string `json:"type"`
	Title  string `json:"title"`
	Detail string `json:"detail,omitempty"`
	Status int    `json:"status"`
}

func writeProblemJSON(w http.ResponseWriter, statusCode int, body any) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	data, _ := json.Marshal(body)
	_, _ = w.Write(data)
}

func writeErrorJSON(w http.ResponseWriter, statusCode int, message string) {
	title := http.StatusText(statusCode)
	p := problemDetails{
		Type:   "about:blank",
		Status: statusCode,
		Title:  title,
	}
	if message != title {
		p.Detail = message
	}
	writeProblemJSON(w, statusCode, p)
}

func writeValidationProblemJSON(w http.ResponseWriter, statusCode int, err *validation.Error) {
	w.Header().Set("Content-Type", "application/problem+json")
	w.WriteHeader(statusCode)
	buf := validationErrorBufPool.Get().(*[]byte)
	*buf = (*buf)[:0]
	*buf = append(*buf, `{"type":"https://tyche.dev/problems/validation-error","title":"Request validation failed","status":`...)
	*buf = strconv.AppendInt(*buf, int64(statusCode), 10)
	*buf = append(*buf, `,"detail":"One or more validation errors occurred.","errors":`...)
	if len(err.Problems) == 0 {
		*buf = append(*buf, "[]"...)
	} else {
		*buf = append(*buf, '[')
		for i, prob := range err.Problems {
			if i > 0 {
				*buf = append(*buf, ',')
			}
			*buf = append(*buf, `{"pointer":`...)
			*buf = strconv.AppendQuote(*buf, prob.Pointer)
			*buf = append(*buf, `,"code":`...)
			*buf = strconv.AppendQuote(*buf, prob.Code)
			*buf = append(*buf, `,"message":`...)
			*buf = strconv.AppendQuote(*buf, prob.Message)
			*buf = append(*buf, '}')
		}
		*buf = append(*buf, ']')
	}
	*buf = append(*buf, '}')
	_, _ = w.Write(*buf)
	if cap(*buf) > 4096 {
		*buf = make([]byte, 0, 512)
	}
	validationErrorBufPool.Put(buf)
}

type writtenChecker interface {
	Written() bool
}

type trackedResponseWriter struct {
	http.ResponseWriter
	written bool
}

func (w *trackedResponseWriter) WriteHeader(statusCode int) {
	w.written = true
	w.ResponseWriter.WriteHeader(statusCode)
}

func (w *trackedResponseWriter) Write(p []byte) (int, error) {
	w.written = true
	return w.ResponseWriter.Write(p)
}

func (w *trackedResponseWriter) Written() bool {
	return w.written
}

func (w *trackedResponseWriter) Flush() {
	if flusher, ok := w.ResponseWriter.(http.Flusher); ok {
		flusher.Flush()
	}
}

func (w *trackedResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hijacker, ok := w.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, fmt.Errorf("response writer does not support hijacking")
	}
	return hijacker.Hijack()
}

func (w *trackedResponseWriter) ReadFrom(r io.Reader) (int64, error) {
	if readerFrom, ok := w.ResponseWriter.(io.ReaderFrom); ok {
		w.written = true
		return readerFrom.ReadFrom(r)
	}
	return io.Copy(w.ResponseWriter, r)
}

func (w *trackedResponseWriter) Push(target string, opts *http.PushOptions) error {
	pusher, ok := w.ResponseWriter.(http.Pusher)
	if !ok {
		return http.ErrNotSupported
	}
	return pusher.Push(target, opts)
}
