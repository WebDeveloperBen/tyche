package server

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"slices"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/webdeveloperben/tyche/server/openapi"
)

// RouteTarget is what Register / RegisterStream register against: a place to
// attach the composed handler (handleRoute) plus the shared OpenAPI/operation
// state to document it in. Both [API] (registering at the root) and [APIGroup]
// (registering under a prefix) satisfy it, so the typed registration functions
// accept either.
type RouteTarget interface {
	// handleRoute composes middleware and body limits around fn and registers
	// the final handler for method+opPath under this target's routing backend.
	handleRoute(method, opPath string, fn HandlerFunc, ro routeOptions) error

	groupPrefix() string
	apiDoc() *openapi.OpenAPI
	apiSchemaRegistry() *openapi.Registry
	apiCodecs() []Codec
	apiOperations() []RegisteredOperation
	recordOperation(RegisteredOperation)
	invalidateOpenAPI()
}

// ---- API: the bring-your-own-router entry point ----

// API is a router-agnostic tyche server. It owns the OpenAPI document, schema
// registry, error handling, and middleware composition, and delegates all path
// matching to an [Adapter]. It implements [RouteTarget], so Register /
// RegisterStream can register directly on it (at the root prefix) or on any
// [APIGroup] it produces.
type API struct {
	dispatch             http.Handler
	adapter              Adapter
	notFound             http.Handler
	methodNotAllowed     http.Handler
	registered           map[string]struct{}
	schemaRegistry       *openapi.Registry
	doc                  *openapi.OpenAPI
	root                 *APIGroup
	errorHandler         ErrorHandler
	operations           []RegisteredOperation
	rootMW               []Middleware
	serveHTTPMiddlewares []ServeHTTPMiddleware
	routes               []*apiRoute
	codecs               []Codec
	openapiJSON          []byte
	maxBodyBytes         int64
	openapiMu            sync.RWMutex
}

// apiRoute is a registered route. Its composed (middleware-wrapped) handler is
// stored behind an atomic pointer so that a late Use (root or group) can
// rebuild every affected chain in place without racing in-flight requests that
// are reading it.
type apiRoute struct {
	group    *APIGroup
	fn       HandlerFunc
	wrapped  atomic.Pointer[HandlerFunc]
	template string
	routeMW  []Middleware
	limit    int64
}

// NewAPI builds an API over the given adapter. Config is optional; the zero
// value yields defaults (title "API", version "1.0.0", 10 MiB body limit).
//
//	api := server.NewAPI(server.NewServeMuxAdapter())                 // defaults
//	api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{ ... })
func NewAPI(adapter Adapter, cfg ...APIConfig) *API {
	var c APIConfig
	if len(cfg) > 0 {
		c = cfg[0]
	}
	merged := mergeAPIConfig(APIConfig{
		OpenAPI:             OpenAPIInfo{Title: "API", Version: "1.0.0"},
		Codecs:              []Codec{JSONCodec{}},
		MaxRequestBodyBytes: 10 << 20,
	}, c)

	a := &API{
		adapter:          adapter,
		errorHandler:     merged.ErrorHandler,
		notFound:         http.HandlerFunc(defaultNotFoundHandler),
		methodNotAllowed: http.HandlerFunc(defaultMethodNotAllowedHandler),
		doc:              openapi.NewOpenAPI(merged.OpenAPI.Title, merged.OpenAPI.Version),
		schemaRegistry:   openapi.NewRegistry("#/components/schemas"),
		codecs:           normalizeCodecs(merged.Codecs),
		maxBodyBytes:     merged.MaxRequestBodyBytes,
		registered:       make(map[string]struct{}),
	}
	if a.errorHandler == nil {
		a.errorHandler = DefaultErrorHandler
	}
	a.doc.Info.Description = merged.OpenAPI.Description
	a.root = &APIGroup{api: a}
	a.adapter.SetFallback(a.notFound, a.methodNotAllowed)
	a.rebuildDispatch()
	return a
}

// Use appends root-level middleware applied to every route and rebuilds the
// affected handler chains. Call during setup, before serving.
func (a *API) Use(mw ...Middleware) {
	a.rootMW = append(a.rootMW, mw...)
	a.rebuildHandlers()
}

// rebuildHandlers recomposes every registered route's middleware chain in
// place. Called after a late Use so middleware registered after a route still
// applies to it.
func (a *API) rebuildHandlers() {
	for _, rt := range a.routes {
		composed := a.compose(rt.fn, rt.group, rt.routeMW)
		rt.wrapped.Store(&composed)
	}
}

// UseHTTP appends [ServeHTTPMiddleware] that wraps the entire router at the
// network edge, before routing — so it observes 404/405 responses and the
// final status. Call during setup, before serving.
func (a *API) UseHTTP(mw ...ServeHTTPMiddleware) {
	a.serveHTTPMiddlewares = append(a.serveHTTPMiddlewares, mw...)
	a.rebuildDispatch()
}

// UseServeHTTP is an alias for UseHTTP accepting a single middleware.
func (a *API) UseServeHTTP(mw ServeHTTPMiddleware) { a.UseHTTP(mw) }

// SetErrorHandler overrides the handler invoked when a route returns an error.
// Passing nil resets it to DefaultErrorHandler.
func (a *API) SetErrorHandler(h ErrorHandler) {
	if h == nil {
		h = DefaultErrorHandler
	}
	a.errorHandler = h
}

// SetNotFoundHandler overrides the handler used when no route matches. Passing
// nil resets it to the default problem+json responder.
func (a *API) SetNotFoundHandler(h http.Handler) {
	if h == nil {
		h = http.HandlerFunc(defaultNotFoundHandler)
	}
	a.notFound = h
	a.adapter.SetFallback(a.notFound, a.methodNotAllowed)
}

// SetMethodNotAllowedHandler overrides the handler used when a route exists for
// the path but not the request method. Passing nil resets the default.
func (a *API) SetMethodNotAllowedHandler(h http.Handler) {
	if h == nil {
		h = http.HandlerFunc(defaultMethodNotAllowedHandler)
	}
	a.methodNotAllowed = h
	a.adapter.SetFallback(a.notFound, a.methodNotAllowed)
}

// Group returns a prefixed, optionally middleware-scoped registration target.
func (a *API) Group(prefix string, mw ...Middleware) *APIGroup {
	return a.root.Group(prefix, mw...)
}

// AddSecurityScheme registers a named security scheme in the OpenAPI document.
func (a *API) AddSecurityScheme(name string, scheme *SecurityScheme) {
	a.doc.AddSecurityScheme(name, scheme)
	a.invalidateOpenAPICache()
}

// OpenAPI returns the underlying document.
func (a *API) OpenAPI() *openapi.OpenAPI { return a.doc }

// SchemaRegistry returns the OpenAPI schema registry.
func (a *API) SchemaRegistry() *openapi.Registry { return a.schemaRegistry }

// RegisteredOperations returns a copy of the operations registered so far.
func (a *API) RegisteredOperations() []RegisteredOperation {
	return append([]RegisteredOperation(nil), a.operations...)
}

// UseNamed registers the Middleware of each NamedMiddleware at the root,
// preserving order.
func (a *API) UseNamed(mws ...NamedMiddleware) {
	for _, m := range mws {
		if m != nil {
			a.Use(m.Middleware())
		}
	}
}

// supportedMethod reports whether method is an HTTP method tyche will route.
func supportedMethod(method string) bool {
	switch method {
	case http.MethodGet, http.MethodHead, http.MethodPost, http.MethodPut,
		http.MethodPatch, http.MethodDelete, http.MethodConnect,
		http.MethodOptions, http.MethodTrace:
		return true
	default:
		return false
	}
}

// ServeHTTP wraps the request in a tracked response writer (for "already
// written" detection) and hands it to the UseHTTP middleware chain, which
// wraps the router dispatch. Per-route body limits, middleware, and error
// rendering are applied by the handler the API registered with the adapter.
func (a *API) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	tracked := trackedResponseWriterPool.Get().(*trackedResponseWriter)
	tracked.ResponseWriter = w
	tracked.written = false
	defer func() {
		tracked.ResponseWriter = nil
		trackedResponseWriterPool.Put(tracked)
	}()
	a.dispatch.ServeHTTP(tracked, r)
}

// route is the innermost dispatch: it rejects path traversal, then delegates to
// the adapter (which handles matching and 404/405 fallback). UseHTTP middleware
// wraps this, so it observes the final response.
func (a *API) route(w http.ResponseWriter, r *http.Request) {
	if HasPathTraversal(r.URL.Path) {
		a.errorHandler(w, r, NewHTTPError(http.StatusBadRequest, "Path traversal not allowed"))
		return
	}
	a.adapter.ServeHTTP(w, r)
}

func (a *API) rebuildDispatch() {
	var h http.Handler = http.HandlerFunc(a.route)
	for _, mw := range slices.Backward(a.serveHTTPMiddlewares) {
		h = mw(h)
	}
	a.dispatch = h
}

// ---- untyped route registration + RouteTarget (root-level) ----

// The API satisfies RouteTarget by delegating to its root group, so typed
// routes can be registered directly on the API as well as on any APIGroup.
func (a *API) handleRoute(method, opPath string, fn HandlerFunc, ro routeOptions) error {
	return a.root.handleRoute(method, opPath, fn, ro)
}
func (a *API) groupPrefix() string                    { return a.root.groupPrefix() }
func (a *API) apiDoc() *openapi.OpenAPI               { return a.doc }
func (a *API) apiSchemaRegistry() *openapi.Registry   { return a.schemaRegistry }
func (a *API) apiCodecs() []Codec                     { return a.Codecs() }
func (a *API) apiOperations() []RegisteredOperation   { return a.operations }
func (a *API) recordOperation(op RegisteredOperation) { a.operations = append(a.operations, op) }
func (a *API) invalidateOpenAPI()                     { a.invalidateOpenAPICache() }

var (
	_ RouteTarget = (*API)(nil)
	_ RouteTarget = (*APIGroup)(nil)
)

func (a *API) HandleE(method, pattern string, fn HandlerFunc, opts ...RouteOption) error {
	return a.root.HandleE(method, pattern, fn, opts...)
}

func (a *API) Handle(method, pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.Handle(method, pattern, fn, opts...)
}

func (a *API) GET(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.GET(pattern, fn, opts...)
}

func (a *API) POST(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.POST(pattern, fn, opts...)
}

func (a *API) PUT(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.PUT(pattern, fn, opts...)
}

func (a *API) DELETE(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.DELETE(pattern, fn, opts...)
}

func (a *API) PATCH(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.PATCH(pattern, fn, opts...)
}

func (a *API) OPTIONS(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.OPTIONS(pattern, fn, opts...)
}

func (a *API) HEAD(pattern string, fn HandlerFunc, opts ...RouteOption) {
	a.root.HEAD(pattern, fn, opts...)
}

// Mount attaches an arbitrary http.Handler at prefix, serving it for the prefix
// and every sub-path beneath it. The handler receives the unmodified request
// path. Routes registered via Mount are not included in the OpenAPI document.
func (a *API) Mount(prefix string, handler http.Handler) error {
	if handler == nil {
		return errors.New("mount handler cannot be nil")
	}
	return a.MountFunc(prefix, handler.ServeHTTP)
}

// MountFunc is the http.HandlerFunc form of Mount.
func (a *API) MountFunc(prefix string, handler http.HandlerFunc) error {
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
	// Register both the exact prefix and the subtree wildcard: a lone trailing
	// wildcard ("/debug/*") matches sub-paths but not the bare prefix
	// ("/debug"), so the exact route covers the prefix itself.
	for _, method := range mountMethods {
		if err := a.root.HandleE(method, prefix, fn); err != nil {
			return err
		}
		if err := a.root.HandleE(method, prefix+"/*", fn); err != nil {
			return err
		}
	}
	return nil
}

// OpenAPIHandler serves the document as JSON (cached after first render).
func (a *API) OpenAPIHandler() HandlerFunc {
	return func(w http.ResponseWriter, req *http.Request) error {
		a.openapiMu.RLock()
		body := a.openapiJSON
		a.openapiMu.RUnlock()
		if body == nil {
			a.openapiMu.Lock()
			if a.openapiJSON == nil {
				b, err := json.Marshal(a.doc)
				if err != nil {
					a.openapiMu.Unlock()
					return err
				}
				a.openapiJSON = b
			}
			body = a.openapiJSON
			a.openapiMu.Unlock()
		}
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		if req.Method == http.MethodHead {
			return nil
		}
		_, err := w.Write(body)
		return err
	}
}

// MountOpenAPI registers the OpenAPI JSON handler at path (GET + HEAD).
func (a *API) MountOpenAPI(path string) error {
	h := a.OpenAPIHandler()
	if err := a.root.handleRoute(http.MethodGet, path, h, routeOptions{}); err != nil {
		return err
	}
	return a.root.handleRoute(http.MethodHead, path, h, routeOptions{})
}

// finalHandler produces the http.Handler registered with the adapter: it sets
// the route template for RoutePattern, applies the body limit, invokes the
// composed tyche handler, and routes any returned error through the handler.
func (a *API) finalHandler(rt *apiRoute) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		r.Pattern = rt.template
		if rt.limit > 0 && r.Body != nil && r.Body != http.NoBody {
			r.Body = http.MaxBytesReader(w, r.Body, rt.limit)
		}
		h := *rt.wrapped.Load()
		if err := h(w, r); err != nil {
			a.errorHandler(w, r, err)
		}
	})
}

func (a *API) compose(fn HandlerFunc, g *APIGroup, routeMW []Middleware) HandlerFunc {
	stack := make([]Middleware, 0, len(a.rootMW)+len(g.middlewareChain()))
	stack = append(stack, a.rootMW...)
	stack = append(stack, g.middlewareChain()...)

	wrapped := fn
	for _, m := range slices.Backward(routeMW) {
		wrapped = m(wrapped)
	}
	for _, m := range slices.Backward(stack) {
		wrapped = m(wrapped)
	}
	return wrapped
}

func (a *API) invalidateOpenAPICache() {
	a.openapiMu.Lock()
	a.openapiJSON = nil
	a.openapiMu.Unlock()
}

// ---- APIGroup implements RouteTarget over the adapter ----

// APIGroup is a prefixed, middleware-scoped registration target backed by an
// [API] and its [Adapter].
type APIGroup struct {
	api    *API
	parent *APIGroup
	prefix string
	stack  []Middleware
}

func (g *APIGroup) Group(prefix string, mw ...Middleware) *APIGroup {
	return &APIGroup{
		api:    g.api,
		parent: g,
		prefix: g.prefix + prefix,
		stack:  append([]Middleware(nil), mw...),
	}
}

func (g *APIGroup) Use(mw ...Middleware) *APIGroup {
	g.stack = append(g.stack, mw...)
	g.api.rebuildHandlers()
	return g
}

// UseNamed registers the Middleware of each NamedMiddleware on the group,
// preserving order. It returns the group for chaining.
func (g *APIGroup) UseNamed(mws ...NamedMiddleware) *APIGroup {
	for _, m := range mws {
		if m != nil {
			g.Use(m.Middleware())
		}
	}
	return g
}

func (g *APIGroup) middlewareChain() []Middleware {
	if g == nil {
		return nil
	}
	var parent []Middleware
	if g.parent != nil {
		parent = g.parent.middlewareChain()
	}
	chain := make([]Middleware, 0, len(parent)+len(g.stack))
	chain = append(chain, parent...)
	chain = append(chain, g.stack...)
	return chain
}

func (g *APIGroup) HandleE(method, pattern string, fn HandlerFunc, opts ...RouteOption) error {
	if g == nil {
		return errors.New("group is required")
	}
	return g.handleRoute(method, pattern, fn, resolveRouteOptions(opts))
}

func (g *APIGroup) Handle(method, pattern string, fn HandlerFunc, opts ...RouteOption) {
	if err := g.HandleE(method, pattern, fn, opts...); err != nil {
		panic(err)
	}
}

func (g *APIGroup) GET(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodGet, pattern, fn, opts...)
}

func (g *APIGroup) POST(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodPost, pattern, fn, opts...)
}

func (g *APIGroup) PUT(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodPut, pattern, fn, opts...)
}

func (g *APIGroup) DELETE(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodDelete, pattern, fn, opts...)
}

func (g *APIGroup) PATCH(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodPatch, pattern, fn, opts...)
}

func (g *APIGroup) OPTIONS(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodOptions, pattern, fn, opts...)
}

func (g *APIGroup) HEAD(pattern string, fn HandlerFunc, opts ...RouteOption) {
	g.Handle(http.MethodHead, pattern, fn, opts...)
}

func (g *APIGroup) handleRoute(method, opPath string, fn HandlerFunc, ro routeOptions) error {
	full := joinPath(g.prefix, opPath)
	// The API layer owns route validation and duplicate detection so the
	// HandleE error contract is identical across adapters, rather than leaking
	// the underlying router's behaviour (e.g. ServeMux panics on duplicates).
	if !strings.HasPrefix(full, "/") {
		return fmt.Errorf("path must start with /: %s", full)
	}
	if !supportedMethod(method) {
		return fmt.Errorf("unsupported method: %s", method)
	}
	if err := validateRoutePattern(full); err != nil {
		return err
	}
	key := method + " " + full
	if _, dup := g.api.registered[key]; dup {
		return fmt.Errorf("duplicate route handler registered for %s %s", method, full)
	}

	limit := g.api.maxBodyBytes
	if ro.maxBodyBytes != nil {
		limit = *ro.maxBodyBytes
	}
	rt := &apiRoute{group: g, fn: fn, routeMW: ro.middleware, limit: limit, template: full}
	composed := g.api.compose(fn, g, ro.middleware)
	rt.wrapped.Store(&composed)

	// Register with the adapter before recording state: the underlying router
	// may reject a pattern by panicking (net/http.ServeMux panics on
	// conflicting or malformed patterns), and HandleE must return that as an
	// error rather than panic or leave a phantom route behind.
	if err := g.api.registerWithAdapter(method, full, rt); err != nil {
		return err
	}
	g.api.registered[key] = struct{}{}
	g.api.routes = append(g.api.routes, rt)
	return nil
}

// registerWithAdapter installs the route on the adapter, converting a router
// panic (e.g. ServeMux's conflicting/malformed-pattern panic) into an error so
// callers of HandleE / RegisterE get the error-returning contract.
func (a *API) registerWithAdapter(method, path string, rt *apiRoute) (err error) {
	defer func() {
		if p := recover(); p != nil {
			err = fmt.Errorf("register route %s %s: %v", method, path, p)
		}
	}()
	a.adapter.Handle(method, path, a.finalHandler(rt))
	return nil
}

func (g *APIGroup) groupPrefix() string                  { return g.prefix }
func (g *APIGroup) apiDoc() *openapi.OpenAPI             { return g.api.doc }
func (g *APIGroup) apiSchemaRegistry() *openapi.Registry { return g.api.schemaRegistry }
func (g *APIGroup) apiCodecs() []Codec                   { return g.api.Codecs() }
func (g *APIGroup) apiOperations() []RegisteredOperation { return g.api.operations }
func (g *APIGroup) recordOperation(op RegisteredOperation) {
	g.api.operations = append(g.api.operations, op)
}
func (g *APIGroup) invalidateOpenAPI() { g.api.invalidateOpenAPICache() }
