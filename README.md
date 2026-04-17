# tyche

Typed Go HTTP routing with OpenAPI generation, pluggable docs UIs, and optional generated request/response codecs.

## What it gives you

- Fast router with static, param, and wildcard paths.
- Typed route registration with OpenAPI output.
- Generated codecs via `servergen` to avoid reflection on supported routes.
- Built-in docs mounting for Scalar and Redoc.
- Middleware support for both route-level and `http.Handler` middleware.

## Recommended public API

Use the error-returning setup APIs in application bootstrap:

```go
router := server.NewRouterWithConfig(server.RouterConfig{
	OpenAPI: server.OpenAPIInfo{
		Title:   "Example API",
		Version: "1.0.0",
	},
	MaxRequestBodyBytes:    10 << 20,
	RequireGeneratedCodecs: true,
})

api := router.Group("/api")

server.Register(api, server.Operation{
	OperationID: "get-user",
	Method:      http.MethodGet,
	Path:        "/users/:id",
}, getUserHandler)

if err := apidocs.Mount(router, apidocs.Config{
	SpecPath: "/openapi.json",
	UIs: []apidocs.UIMount{
		{Path: "/docs", Renderer: apidocs.Scalar()},
	},
}); err != nil {
	return err
}
```

Convenience wrappers such as `server.Register(...)`, `group.Handle(...)`, and `router.GET(...)` still exist, but they panic on invalid setup. They are best treated as must-style helpers.

## CLI

Install:

```sh
go install ./cmd/servergen
```

Use:

```sh
servergen generate ./...
servergen build -o ./bin/api ./cmd/api
servergen run ./cmd/api
servergen test ./...
```

Optional staging excludes can go in `.servergenignore`.

## Benchmarks

Measured on Apple M3 Pro.

Core router path with:

```sh
go test -run '^$' -bench 'BenchmarkRouter_(StaticRoute|ParamRoute|WildcardRoute|ParamLookup|WithMiddleware)' -benchmem ./server
```

| Benchmark | ns/op | B/op | allocs/op |
| --- | ---: | ---: | ---: |
| `BenchmarkRouter_StaticRoute` | 98.21 | 88 | 2 |
| `BenchmarkRouter_ParamRoute` | 164.2 | 112 | 3 |
| `BenchmarkRouter_WildcardRoute` | 120.8 | 88 | 2 |
| `BenchmarkRouter_ParamLookup` | 224.4 | 112 | 3 |
| `BenchmarkRouter_WithMiddleware` | 98.95 | 88 | 2 |

Typed route benchmarks are written as normal fixture routes and regenerated through `servergen` before benchmarking:

```sh
task benchmark
```

That task:

- regenerates the benchmark fixture codecs from normal typed route declarations under `internal/benchmarkfixtures/...`
- runs the router and typed-route benchmark suite against the generated output

Cross-framework comparison benchmarks can be run with:

```sh
task benchmark:comparison
```

The practical takeaway is:

- the plain router path is already fast without codegen
- generated codecs are benchmarked through the same ergonomics developers use in normal route code
- codegen becomes clearly worthwhile once request binding is non-trivial
- the more params, query/header parsing, and JSON body work involved, the more codegen pays off

Treat these as baseline snapshots for regression checking, not a cross-framework shootout.
