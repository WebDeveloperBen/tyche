package server

import (
	"bufio"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/webdeveloperben/tyche/server/openapi"
	"github.com/webdeveloperben/tyche/server/validation"
)

var trackedResponseWriterPool = sync.Pool{
	New: func() any {
		return &trackedResponseWriter{}
	},
}

type Middleware func(next HandlerFunc) HandlerFunc

type HandlerFunc func(http.ResponseWriter, *http.Request) error

func (f HandlerFunc) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f(w, r)
}

type HTTPError struct {
	StatusCode int
	Message    string
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

func (g *Group) Use(mw ...Middleware) *Group {
	g.stack = append(g.stack, mw...)
	g.router.rebuildHandlers()
	return g
}

func (g *Group) HandleE(method, pattern string, fn HandlerFunc) error {
	if g == nil {
		return errors.New("group is required")
	}
	path := joinPath(g.prefix, pattern)
	return g.router.handle(path, method, fn, g)
}

func (g *Group) Handle(method, pattern string, fn HandlerFunc) {
	if err := g.HandleE(method, pattern, fn); err != nil {
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

func (g *Group) GET(pattern string, fn HandlerFunc) { g.Handle(http.MethodGet, pattern, fn) }

func (g *Group) POST(pattern string, fn HandlerFunc) { g.Handle(http.MethodPost, pattern, fn) }

func (g *Group) PUT(pattern string, fn HandlerFunc) { g.Handle(http.MethodPut, pattern, fn) }

func (g *Group) DELETE(pattern string, fn HandlerFunc) { g.Handle(http.MethodDelete, pattern, fn) }

func (g *Group) PATCH(pattern string, fn HandlerFunc) { g.Handle(http.MethodPatch, pattern, fn) }

func (g *Group) OPTIONS(pattern string, fn HandlerFunc) { g.Handle(http.MethodOptions, pattern, fn) }

func (g *Group) HEAD(pattern string, fn HandlerFunc) { g.Handle(http.MethodHead, pattern, fn) }

type ServeHTTPMiddleware func(next http.Handler) http.Handler

type OpenAPIInfo struct {
	Title       string
	Description string
	Version     string
}

type RouterConfig struct {
	OpenAPI                OpenAPIInfo
	MaxRequestBodyBytes    int64
	RequireGeneratedCodecs bool
}

type Router struct {
	root                   *node
	rootGroup              *Group
	notFound               http.Handler
	serveHTTPMiddlewares   []ServeHTTPMiddleware
	serveHTTPHandler       http.Handler
	openapiDoc             *openapi.OpenAPI
	openapiJSON            []byte
	openapiMu              sync.RWMutex
	schemaRegistry         *openapi.Registry
	operations             []RegisteredOperation
	maxRequestBodyBytes    int64
	requireGeneratedCodecs bool
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
		root:                   &node{part: "/"},
		notFound:               http.HandlerFunc(http.NotFound),
		openapiDoc:             openapi.NewOpenAPI(merged.OpenAPI.Title, merged.OpenAPI.Version),
		schemaRegistry:         openapi.NewRegistry("#/components/schemas"),
		operations:             make([]RegisteredOperation, 0),
		maxRequestBodyBytes:    merged.MaxRequestBodyBytes,
		requireGeneratedCodecs: merged.RequireGeneratedCodecs,
	}
	r.openapiDoc.Info.Description = merged.OpenAPI.Description
	r.rootGroup = &Group{router: r}
	r.rebuildServeHTTPHandler()
	return r
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
	if override.RequireGeneratedCodecs {
		base.RequireGeneratedCodecs = true
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

func (r *Router) HandleE(method, pattern string, fn HandlerFunc) error {
	return r.rootGroup.HandleE(method, pattern, fn)
}

func (r *Router) handle(pattern, method string, fn HandlerFunc, g *Group) error {
	if !strings.HasPrefix(pattern, "/") {
		return fmt.Errorf("path must start with /: %s", pattern)
	}
	if methodIndex(method) < 0 {
		return fmt.Errorf("unsupported method: %s", method)
	}
	if err := validateRoutePattern(pattern); err != nil {
		return err
	}

	params := extractParams(pattern)
	routeNode := r.root.addRoute(pattern)
	if err := routeNode.setHandler(method, pattern, fn, params, routeNode.wcName, g, r.wrapHandler(fn, g)); err != nil {
		return err
	}
	return nil
}

func validateRoutePattern(pattern string) error {
	parts := splitRouteFast(pattern)
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

func hasPathTraversal(path string) bool {
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
	for _, part := range splitRouteFast(pattern) {
		if len(part) > 0 && part[0] == ':' {
			params = append(params, part[1:])
		}
	}
	return params
}

func (r *Router) GET(pattern string, fn HandlerFunc) { r.rootGroup.Handle(http.MethodGet, pattern, fn) }

func (r *Router) POST(pattern string, fn HandlerFunc) {
	r.rootGroup.Handle(http.MethodPost, pattern, fn)
}

func (r *Router) PUT(pattern string, fn HandlerFunc) { r.rootGroup.Handle(http.MethodPut, pattern, fn) }

func (r *Router) DELETE(pattern string, fn HandlerFunc) {
	r.rootGroup.Handle(http.MethodDelete, pattern, fn)
}

func (r *Router) PATCH(pattern string, fn HandlerFunc) {
	r.rootGroup.Handle(http.MethodPatch, pattern, fn)
}

func (r *Router) OPTIONS(pattern string, fn HandlerFunc) {
	r.rootGroup.Handle(http.MethodOptions, pattern, fn)
}

func (r *Router) HEAD(pattern string, fn HandlerFunc) {
	r.rootGroup.Handle(http.MethodHead, pattern, fn)
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

	if hasPathTraversal(path) {
		handleHTTPError(w, NewHTTPError(http.StatusBadRequest, "Path traversal not allowed"))
		return
	}

	result := r.root.find(req.Method, path)

	if !result.routeExists {
		r.notFound.ServeHTTP(w, req)
		return
	}

	if !result.methodAllowed {
		handleHTTPError(w, NewHTTPError(http.StatusMethodNotAllowed, "Method Not Allowed"))
		return
	}

	handler := result.handler

	paramValues := result.paramValues

	newReq := req
	if r.maxRequestBodyBytes > 0 && req.Body != nil && req.Body != http.NoBody {
		req.Body = http.MaxBytesReader(w, req.Body, r.maxRequestBodyBytes)
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
	if handler.wrappedFn != nil {
		fn = handler.wrappedFn
	}

	if err := fn(w, newReq); err != nil {
		handleHTTPError(w, err)
	}

	if len(paramValues) > 0 {
		putParamValues(paramValues)
	}
}

type writtenChecker interface {
	Written() bool
}

func handleHTTPError(w http.ResponseWriter, err error) {
	var validationErr *validation.Error
	if errors.As(err, &validationErr) {
		if wc, ok := w.(writtenChecker); ok && wc.Written() {
			return
		}
		writeValidationProblemJSON(w, http.StatusBadRequest, validationErr)
		return
	}
	if httpErr, ok := err.(HTTPError); ok {
		if httpErr.Silent {
			return
		}
		if wc, ok := w.(writtenChecker); ok && wc.Written() {
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
	if wc, ok := w.(writtenChecker); ok && wc.Written() {
		return
	}
	writeErrorJSON(w, http.StatusInternalServerError, http.StatusText(http.StatusInternalServerError))
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

func writeErrorJSON(w http.ResponseWriter, statusCode int, message string) {
	_ = WriteJSON(w, statusCode, map[string]string{"error": message})
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
			*buf = append(*buf, '}', '}')
		}
		*buf = append(*buf, ']')
	}
	*buf = append(*buf, '}')
	w.Write(*buf)
	if cap(*buf) > 4096 {
		*buf = make([]byte, 0, 256)
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

func (r *Router) wrapHandler(fn HandlerFunc, g *Group) HandlerFunc {
	stack := g.middlewareChain()
	wrapped := fn
	for i := len(stack) - 1; i >= 0; i-- {
		wrapped = stack[i](wrapped)
	}
	return wrapped
}

func (r *Router) rebuildHandlers() {
	r.root.rebuildHandlers(func(handler *routeHandler) HandlerFunc {
		return r.wrapHandler(handler.fn, handler.group)
	})
}

func (r *Router) rebuildServeHTTPHandler() {
	var handler http.Handler = http.HandlerFunc(r.serveHTTP)
	for i := len(r.serveHTTPMiddlewares) - 1; i >= 0; i-- {
		handler = r.serveHTTPMiddlewares[i](handler)
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
	var lineage []*Group
	for current := g; current != nil; current = current.parent {
		lineage = append(lineage, current)
	}
	var total int
	for _, group := range lineage {
		total += len(group.stack)
	}
	chain := make([]Middleware, 0, total)
	for i := len(lineage) - 1; i >= 0; i-- {
		chain = append(chain, lineage[i].stack...)
	}
	return chain
}

func (r *Router) RequireGeneratedCodecs(required bool) {
	r.requireGeneratedCodecs = required
}
