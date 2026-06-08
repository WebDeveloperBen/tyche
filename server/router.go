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
	"slices"
	"strconv"
	"strings"
	"sync"

	"github.com/webdeveloperben/tyche/server/openapi"
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
	// sink; the error is intentionally discarded. Use it through the router to
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

type Group struct {
	router *Router
	parent *Group
	prefix string
	stack  []Middleware
}

func (g *Group) Group(prefix string, mw ...Middleware) *Group {
	newGroup := &Group{
		router: g.router,
		parent: g,
		prefix: g.prefix + prefix,
		stack:  append([]Middleware(nil), mw...),
	}
	return newGroup
}

// Use appends middleware to the group and rebuilds the affected handlers.
//
// Like all route and middleware registration, Use must be called during setup,
// before the router begins serving requests: it rebuilds handler chains in
// place and is not safe to call concurrently with active request handling.
func (g *Group) Use(mw ...Middleware) *Group {
	g.stack = append(g.stack, mw...)
	g.router.rebuildHandlers()
	return g
}

func (g *Group) HandleE(method, pattern string, fn HandlerFunc, opts ...RouteOption) error {
	if g == nil {
		return errors.New("group is required")
	}
	path := joinPath(g.prefix, pattern)
	ro := resolveRouteOptions(opts)
	return g.router.handle(path, method, fn, g, ro)
}

func (g *Group) Handle(method, pattern string, fn HandlerFunc, opts ...RouteOption) {
	if err := g.HandleE(method, pattern, fn, opts...); err != nil {
		panic(err)
	}
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

func (g *Group) GET(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodGet, pattern, fn, opts...)
}

func (g *Group) POST(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodPost, pattern, fn, opts...)
}

func (g *Group) PUT(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodPut, pattern, fn, opts...)
}

func (g *Group) DELETE(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodDelete, pattern, fn, opts...)
}

func (g *Group) PATCH(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodPatch, pattern, fn, opts...)
}

func (g *Group) OPTIONS(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodOptions, pattern, fn, opts...)
}

func (g *Group) HEAD(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodHead, pattern, fn, opts...)
}

type ServeHTTPMiddleware func(next http.Handler) http.Handler

type OpenAPIInfo struct {
	Title       string
	Description string
	Version     string
}

type RouterConfig struct {
	ErrorHandler        ErrorHandler
	OpenAPI             OpenAPIInfo
	MaxRequestBodyBytes int64
}

// ErrorHandler converts an error returned by a HandlerFunc (or produced by the
// router, e.g. path traversal) into an HTTP response. Implementations should
// respect any response already written; see DefaultErrorHandler.
type ErrorHandler func(w http.ResponseWriter, r *http.Request, err error)

type Router struct {
	serveHTTPHandler     http.Handler
	methodNotAllowed     http.Handler
	notFound             http.Handler
	schemaRegistry       *openapi.Registry
	errorHandler         ErrorHandler
	rootGroup            *Group
	Root                 *node
	openapiDoc           *openapi.OpenAPI
	serveHTTPMiddlewares []ServeHTTPMiddleware
	openapiJSON          []byte
	operations           []RegisteredOperation
	maxRequestBodyBytes  int64
	openapiMu            sync.RWMutex
}

func NewRouter() *Router {
	return NewRouterWithConfig(RouterConfig{})
}

func NewRouterWithConfig(cfg RouterConfig) *Router {
	merged := RouterConfig{
		OpenAPI: OpenAPIInfo{
			Title:   "API",
			Version: "1.0.0",
		},
		MaxRequestBodyBytes: 10 << 20,
	}
	merged = mergeRouterConfig(merged, cfg)

	r := &Router{
		Root:                &node{part: "/"},
		notFound:            http.HandlerFunc(defaultNotFoundHandler),
		methodNotAllowed:    http.HandlerFunc(defaultMethodNotAllowedHandler),
		errorHandler:        merged.ErrorHandler,
		openapiDoc:          openapi.NewOpenAPI(merged.OpenAPI.Title, merged.OpenAPI.Version),
		schemaRegistry:      openapi.NewRegistry("#/components/schemas"),
		operations:          make([]RegisteredOperation, 0),
		maxRequestBodyBytes: merged.MaxRequestBodyBytes,
	}
	if r.errorHandler == nil {
		r.errorHandler = DefaultErrorHandler
	}
	r.openapiDoc.Info.Description = merged.OpenAPI.Description
	r.rootGroup = &Group{router: r}
	r.rebuildServeHTTPHandler()
	return r
}

// SetErrorHandler overrides the handler invoked when a route returns an error.
// Passing nil resets it to DefaultErrorHandler.
func (r *Router) SetErrorHandler(h ErrorHandler) {
	if h == nil {
		h = DefaultErrorHandler
	}
	r.errorHandler = h
}

// SetNotFoundHandler overrides the handler used when no route matches the
// request path. Passing nil resets it to the default problem+json responder.
func (r *Router) SetNotFoundHandler(h http.Handler) {
	if h == nil {
		h = http.HandlerFunc(defaultNotFoundHandler)
	}
	r.notFound = h
}

// SetMethodNotAllowedHandler overrides the handler used when a route exists for
// the path but not for the request method. Passing nil resets it to the default
// problem+json responder.
func (r *Router) SetMethodNotAllowedHandler(h http.Handler) {
	if h == nil {
		h = http.HandlerFunc(defaultMethodNotAllowedHandler)
	}
	r.methodNotAllowed = h
}

func mergeRouterConfig(base, override RouterConfig) RouterConfig {
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

func (r *Router) Group(prefix string, mw ...Middleware) *Group {
	g := &Group{
		router: r,
		parent: r.rootGroup,
		prefix: prefix,
		stack:  append([]Middleware(nil), mw...),
	}
	return g
}

// Use appends root-level middleware applied to every route. It must be called
// during setup, before the router serves requests (it rebuilds handler chains
// in place and is not safe to call concurrently with active request handling).
func (r *Router) Use(middleware ...Middleware) {
	r.rootGroup.stack = append(r.rootGroup.stack, middleware...)
	r.rebuildHandlers()
}

func (r *Router) UseHTTP(middleware ...ServeHTTPMiddleware) {
	r.serveHTTPMiddlewares = append(r.serveHTTPMiddlewares, middleware...)
	r.rebuildServeHTTPHandler()
}

func (r *Router) UseServeHTTP(mw ServeHTTPMiddleware) {
	r.UseHTTP(mw)
}

func (r *Router) HandleE(method, pattern string, fn HandlerFunc, opts ...RouteOption) error {
	return r.rootGroup.HandleE(method, pattern, fn, opts...)
}

func (r *Router) handle(pattern, method string, fn HandlerFunc, g *Group, ro routeOptions) error {
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("path must start with /: %s", pattern)
	}
	if MethodIndex(method) < 0 {
		return fmt.Errorf("unsupported method: %s", method)
	}
	if err := validateRoutePattern(pattern); err != nil {
		return err
	}

	params := extractParams(pattern)
	routeNode := r.Root.addRoute(pattern)
	if err := routeNode.setHandler(method, pattern, fn, params, routeNode.wcName, g, ro.middleware, ro.maxBodyBytes, r.wrapHandler(fn, g, ro.middleware)); err != nil {
		return err
	}
	return nil
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

func extractParams(pattern string) []string {
	var params []string
	for _, part := range SplitRouteFast(pattern) {
		if len(part) > 0 && part[0] == ':' {
			params = append(params, part[1:])
		}
	}
	return params
}

func (r *Router) GET(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodGet, pattern, fn, opts...)
}

func (r *Router) POST(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodPost, pattern, fn, opts...)
}

func (r *Router) PUT(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodPut, pattern, fn, opts...)
}

func (r *Router) DELETE(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodDelete, pattern, fn, opts...)
}

func (r *Router) PATCH(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodPatch, pattern, fn, opts...)
}

func (r *Router) OPTIONS(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodOptions, pattern, fn, opts...)
}

func (r *Router) HEAD(pattern string, fn HandlerFunc, opts ...RouteOption) {
	r.rootGroup.Handle(http.MethodHead, pattern, fn, opts...)
}

func (r *Router) OpenAPIHandler() HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) error {
		body, err := r.openAPIJSON()
		if err != nil {
			return err
		}

		w.Header().Set("Content-Type", "application/json")
		if req.Method == http.MethodHead {
			w.WriteHeader(http.StatusOK)
			return nil
		}

		w.WriteHeader(http.StatusOK)
		_, err = w.Write(body)
		return err
	}
}

func (r *Router) MountOpenAPI(path string) error {
	handler := r.OpenAPIHandler()
	if err := r.HandleE(http.MethodGet, path, handler); err != nil {
		return err
	}

	return r.HandleE(http.MethodHead, path, handler)
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

// Mount attaches an arbitrary [http.Handler] at prefix, serving it for the
// prefix itself and every sub-path beneath it. It is useful for embedding
// third-party handlers such as net/http/pprof, a metrics endpoint, or another
// mux:
//
//	router.Mount("/debug/pprof", pprofMux)
//
// The handler is registered for GET, HEAD, POST, PUT, PATCH, DELETE, and
// OPTIONS (not CONNECT or TRACE). It receives the unmodified request path (the
// prefix is not stripped), matching the expectations of handlers like
// net/http/pprof. Routes registered via Mount are not included in the OpenAPI
// document.
//
// A concrete route that overlaps the mount prefix takes precedence for the
// exact paths and methods it registers; mount the handler on a dedicated prefix
// to avoid surprising shadowing.
func (r *Router) Mount(prefix string, handler http.Handler) error {
	if handler == nil {
		return errors.New("mount handler cannot be nil")
	}
	return r.MountFunc(prefix, handler.ServeHTTP)
}

// MountFunc is the [http.HandlerFunc] form of [Router.Mount].
func (r *Router) MountFunc(prefix string, handler http.HandlerFunc) error {
	if handler == nil {
		return errors.New("mount handler cannot be nil")
	}
	if !strings.HasPrefix(prefix, "/") {
		return fmt.Errorf("mount prefix must start with /: %s", prefix)
	}
	prefix = strings.TrimRight(prefix, "/")
	if prefix == "" {
		return errors.New(`mount prefix cannot be empty or "/"`)
	}

	fn := func(w http.ResponseWriter, req *http.Request) error {
		handler.ServeHTTP(w, req)
		return nil
	}

	// A single wildcard registration serves both the exact prefix and every
	// sub-path: the router stores the wildcard handler on the prefix node,
	// which also answers an exact-prefix request.
	pattern := prefix + "/*"
	for _, method := range mountMethods {
		if err := r.HandleE(method, pattern, fn); err != nil {
			return err
		}
	}
	return nil
}

func (r *Router) ServeHTTP(w http.ResponseWriter, req *http.Request) {
	tracked := trackedResponseWriterPool.Get().(*trackedResponseWriter)

	tracked.ResponseWriter = w

	tracked.written = false

	defer func() {
		tracked.ResponseWriter = nil
		trackedResponseWriterPool.Put(tracked)
	}()

	if len(r.serveHTTPMiddlewares) == 0 {
		r.serveHTTP(tracked, req)
		return
	}

	r.serveHTTPHandler.ServeHTTP(tracked, req)
}

func (r *Router) serveHTTP(w http.ResponseWriter, req *http.Request) {
	path := req.URL.Path

	if HasPathTraversal(path) {
		r.errorHandler(w, req, NewHTTPError(http.StatusBadRequest, "Path traversal not allowed"))
		return
	}

	result := r.Root.Find(req.Method, path)

	if !result.routeExists {
		r.notFound.ServeHTTP(w, req)
		return
	}

	if !result.methodAllowed {
		r.methodNotAllowed.ServeHTTP(w, req)
		return
	}

	handler := result.handler

	paramValues := result.paramValues

	newReq := req
	// Record the matched route template on the standard net/http Pattern field
	// (zero-allocation) so middleware and handlers can read a low-cardinality
	// route label via RoutePattern. The router does not otherwise use it.
	req.Pattern = handler.route
	bodyLimit := r.maxRequestBodyBytes
	if handler.maxBodyBytes != nil {
		bodyLimit = *handler.maxBodyBytes
	}
	if bodyLimit > 0 && req.Body != nil && req.Body != http.NoBody {
		req.Body = http.MaxBytesReader(w, req.Body, bodyLimit)
	}

	if len(paramValues) > 0 {
		for i, value := range paramValues {
			req.SetPathValue(handler.params[len(handler.params)-1-i], value)
		}
	}

	if result.hasWildcard {
		req.SetPathValue("*", result.wildcard)
		if handler.wildcard != "" {
			req.SetPathValue(handler.wildcard, result.wildcard)
		}
	}

	fn := handler.fn
	if wf := handler.wrappedFn.Load(); wf != nil {
		fn = *wf
	}

	if err := fn(w, newReq); err != nil {
		r.errorHandler(w, newReq, err)
	}

	if len(paramValues) > 0 {
		putParamValues(paramValues)
	}
}

type writtenChecker interface {
	Written() bool
}

// DefaultErrorHandler is the [ErrorHandler] used when a router is not given a
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

// wrapHandler builds the final handler for a route by applying middleware from
// outermost to innermost: root middleware, then group middleware (parent to
// child via middlewareChain), then route-level middleware, then the handler.
func (r *Router) wrapHandler(fn HandlerFunc, g *Group, routeMW []Middleware) HandlerFunc {
	stack := g.middlewareChain()
	wrapped := fn
	for _, r := range slices.Backward(routeMW) {
		wrapped = r(wrapped)
	}
	for _, s := range slices.Backward(stack) {
		wrapped = s(wrapped)
	}
	return wrapped
}

func (r *Router) rebuildHandlers() {
	r.Root.rebuildHandlers(func(handler *routeHandler) HandlerFunc {
		return r.wrapHandler(handler.fn, handler.group, handler.routeMW)
	})
}

func (r *Router) rebuildServeHTTPHandler() {
	var handler http.Handler = http.HandlerFunc(r.serveHTTP)
	for _, v := range slices.Backward(r.serveHTTPMiddlewares) {
		handler = v(handler)
	}
	r.serveHTTPHandler = handler
}

func (r *Router) openAPIJSON() ([]byte, error) {
	r.openapiMu.RLock()
	if r.openapiJSON != nil {
		body := r.openapiJSON
		r.openapiMu.RUnlock()
		return body, nil
	}
	r.openapiMu.RUnlock()

	r.openapiMu.Lock()
	defer r.openapiMu.Unlock()
	if r.openapiJSON != nil {
		return r.openapiJSON, nil
	}
	body, err := json.Marshal(r.OpenAPI())
	if err != nil {
		return nil, err
	}
	r.openapiJSON = body
	return body, nil
}

func (r *Router) invalidateOpenAPICache() {
	r.openapiMu.Lock()
	r.openapiJSON = nil
	r.openapiMu.Unlock()
}

func (g *Group) middlewareChain() []Middleware {
	if g == nil {
		return nil
	}
	var parentChain []Middleware
	if g.parent != nil {
		parentChain = g.parent.middlewareChain()
	}
	chain := make([]Middleware, 0, len(parentChain)+len(g.stack))
	chain = append(chain, parentChain...)
	chain = append(chain, g.stack...)
	return chain
}
