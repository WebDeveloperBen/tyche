// Package server provides typed HTTP handlers, OpenAPI generation, and a
// bring-your-own-router adapter model. An [API] owns request binding,
// validation, response serialization, middleware, and the OpenAPI document,
// and delegates path matching to a pluggable [Adapter] — the standard-library
// [ServeMuxAdapter] by default, or any router you wrap. Route patterns support
// params (:id) and a trailing wildcard (*path).
//
// Beyond routing it provides:
//
//   - Middleware at root, group, and route scope (see Middleware, APIGroup.Use,
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
