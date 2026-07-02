// Package plugins provides production-ready middleware for tyche APIs —
// recoverer, request ID, real IP, logging, timeout, rate limiting, CORS,
// security headers, gzip/brotli compression, and instrumentation — applied with
// api.Use (handler middleware) or api.UseHTTP (edge middleware). The rate
// limiter returns a struct so its refill goroutine can be stopped via Stop.
package plugins
