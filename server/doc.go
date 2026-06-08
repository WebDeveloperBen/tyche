// Package server provides HTTP server construction and routing using the standard library.
// It offers configurable server setup, graceful shutdown, route groups, middleware,
// and a radix tree router supporting params (:id) and wildcards (*path).
//
// Beyond basic routing it provides:
//
//   - Middleware at root, group, and route scope (see Middleware, Group.Use,
//     WithMiddleware, Chain, and MiddlewareFromFunc).
//   - Typed route registration with OpenAPI output (Register) and typed
//     Server-Sent Events streaming (RegisterStream, EventStream, Stream).
//   - Pluggable error handling (ErrorHandler, DefaultErrorHandler) and custom
//     not-found / method-not-allowed handlers.
//   - Per-route request body limits (WithMaxBodyBytes) and mounting of arbitrary
//     http.Handlers (Mount).
//   - OpenAPI security schemes (AddSecurityScheme, BearerScheme, APIKeyScheme,
//     BasicScheme) and per-operation security requirements (Operation.Security).
//   - Typed request-scoped context keys (NewContextKey) and the matched route
//     template (RoutePattern) for logging and metrics.
package server
