package server

import (
	"context"
	"net/http"
	"slices"
)

// MiddlewareFunc is an inline middleware signature that receives the next
// handler alongside the request, removing the double-closure boilerplate of
// the bare [Middleware] type. Convert it with [MiddlewareFromFunc].
type MiddlewareFunc func(w http.ResponseWriter, r *http.Request, next HandlerFunc) error

// MiddlewareFromFunc adapts a [MiddlewareFunc] into a [Middleware]. It lets
// middleware authors write a single function body instead of nesting two
// closures:
//
//	func RequestID() server.Middleware {
//		return server.MiddlewareFromFunc(func(
//			w http.ResponseWriter,
//			r *http.Request,
//			next server.HandlerFunc,
//		) error {
//			ctx := context.WithValue(r.Context(), requestIDKey{}, newRequestID())
//			return next(w, r.WithContext(ctx))
//		})
//	}
func MiddlewareFromFunc(fn MiddlewareFunc) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			return fn(w, r, next)
		}
	}
}

// Chain composes multiple middleware into a single [Middleware]. The first
// middleware in the list runs outermost (closest to the network) and the last
// runs innermost (closest to the handler), matching the order they would run
// if applied individually via [Group.Use]. It is handy for packaging a named
// group of middleware for reuse:
//
//	api.Use(server.Chain(
//		middleware.RequestID(),
//		middleware.Auth(authSvc),
//		middleware.AccessLog(logger),
//	))
func Chain(mw ...Middleware) Middleware {
	return func(next HandlerFunc) HandlerFunc {
		for _, m := range slices.Backward(mw) {
			next = m(next)
		}
		return next
	}
}

// NamedMiddleware is a middleware that carries a stable identity. It fits a
// plugin-style architecture where middleware are discovered and registered
// dynamically, and the name is useful for logging, ordering, or deduplication.
type NamedMiddleware interface {
	Name() string
	Middleware() Middleware
}

// UseNamed is defined on API and APIGroup (see api.go).

// routeOptions accumulates per-route configuration supplied via [RouteOption]
// values when registering a single route.
type routeOptions struct {
	maxBodyBytes         *int64
	middleware           []Middleware
	requestContentTypes  []string
	responseContentTypes []string
}

// RouteOption customizes the registration of an individual route. Options are
// accepted by the verb helpers ([Group.GET], [Group.POST], ...), [Group.Handle],
// [Group.HandleE], and the typed [Register]/[RegisterE] functions.
type RouteOption func(*routeOptions)

// WithMiddleware attaches middleware to a single route. The middleware run
// after any root- and group-level middleware and immediately before the
// handler, in the order given:
//
//	server.Register(api, op, handler,
//		server.WithMiddleware(middleware.RequireScope("llm:chat")),
//	)
func WithMiddleware(mw ...Middleware) RouteOption {
	return func(o *routeOptions) {
		o.middleware = append(o.middleware, mw...)
	}
}

// WithMaxBodyBytes overrides the router-wide request body size limit
// ([APIConfig.MaxRequestBodyBytes]) for a single route. A positive value
// caps the body at that many bytes; a value of 0 removes the limit for the
// route entirely. This is useful when most endpoints want a small default but a
// specific endpoint (a file upload, a large model prompt) needs a different
// ceiling:
//
//	server.Register(api, uploadOp, uploadHandler,
//		server.WithMaxBodyBytes(100<<20), // 100 MiB
//	)
func WithMaxBodyBytes(n int64) RouteOption {
	return func(o *routeOptions) {
		v := n
		o.maxBodyBytes = &v
	}
}

// WithRequestContentTypes restricts a typed route's non-multipart request body
// codecs to the given media types. Each media type must be registered in
// [APIConfig.Codecs]. Routes without this option allow every configured codec.
func WithRequestContentTypes(mediaTypes ...string) RouteOption {
	return func(o *routeOptions) {
		o.requestContentTypes = append(o.requestContentTypes, mediaTypes...)
	}
}

// WithResponseContentTypes restricts a typed route's successful response body
// codecs to the given media types. Each media type must be registered in
// [APIConfig.Codecs]. Routes without this option allow every configured codec.
func WithResponseContentTypes(mediaTypes ...string) RouteOption {
	return func(o *routeOptions) {
		o.responseContentTypes = append(o.responseContentTypes, mediaTypes...)
	}
}

func resolveRouteOptions(opts []RouteOption) routeOptions {
	var ro routeOptions
	for _, opt := range opts {
		if opt != nil {
			opt(&ro)
		}
	}
	return ro
}

// ContextKey is a typed key for storing and retrieving a value of type T on a
// [context.Context]. It removes the boilerplate of declaring an unexported key
// type plus paired getter/setter helpers for request-scoped metadata such as
// auth claims, trace identifiers, or tenant information:
//
//	var authKey = server.NewContextKey[Claims]("auth")
//
//	// in middleware
//	r = authKey.WithRequest(r, claims)
//
//	// in a handler
//	claims, ok := authKey.From(r.Context())
//
// Each call to [NewContextKey] produces a distinct key, so values stored under
// different keys never collide even when T is identical.
type ContextKey[T any] struct {
	k *contextKey
}

type contextKey struct {
	name string
}

// NewContextKey returns a new, unique [ContextKey] for values of type T. The
// name is used only for debugging via the key's String method and need not be
// unique.
func NewContextKey[T any](name string) ContextKey[T] {
	return ContextKey[T]{k: &contextKey{name: name}}
}

func (c *contextKey) String() string {
	return "tyche context key " + c.name
}

// WithValue returns a copy of ctx carrying value associated with this key.
func (key ContextKey[T]) WithValue(ctx context.Context, value T) context.Context {
	return context.WithValue(ctx, key.k, value)
}

// WithRequest returns a shallow copy of r whose context carries value
// associated with this key. It is the request-oriented counterpart to
// [ContextKey.WithValue].
func (key ContextKey[T]) WithRequest(r *http.Request, value T) *http.Request {
	return r.WithContext(key.WithValue(r.Context(), value))
}

// From returns the value associated with this key on ctx and reports whether a
// value of type T was present.
func (key ContextKey[T]) From(ctx context.Context) (T, bool) {
	value, ok := ctx.Value(key.k).(T)
	return value, ok
}
