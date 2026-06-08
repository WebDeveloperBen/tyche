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

## Middleware

Tyche middleware is a `func(next HandlerFunc) HandlerFunc`. It can be applied at three scopes, which run outermost to innermost: root, group, then route.

```go
// http.Handler middleware runs at the network edge, before routing.
router.UseHTTP(httpmw.Recover(), httpmw.RealIP())

// Root- and group-level Tyche middleware.
router.Use(middleware.RequestID())
api := router.Group("/v1", middleware.Auth(authSvc), middleware.AccessLog(logger))

// Route-level middleware via WithMiddleware. Runs after root and group
// middleware, immediately before the handler.
server.Register(api, chatOp, chatHandler,
	server.WithMiddleware(middleware.RequireScope("llm:chat")),
)

api.POST("/v1/chat/completions", handler,
	server.WithMiddleware(middleware.RequireAuth()),
)
```

Ergonomic helpers:

- `server.MiddlewareFromFunc(fn)` — write middleware as a single `func(w, r, next) error` instead of nesting two closures.
- `server.Chain(mw...)` — compose several middleware into one, for packaging reusable groups.
- `server.WithMiddleware(mw...)` — attach middleware to a single route (accepted by the verb helpers, `Handle`/`HandleE`, and `Register`/`RegisterE`).
- `server.NamedMiddleware` + `router.UseNamed(...)` / `group.UseNamed(...)` — register plugin-style middleware that carries a stable name.
- `server.NewContextKey[T](name)` — a typed context key with `WithValue`/`WithRequest`/`From` for request-scoped metadata (auth claims, trace IDs, tenant info) without hand-rolling unexported key types.

```go
var authKey = server.NewContextKey[Claims]("auth")

func Auth(svc AuthService) server.Middleware {
	return server.MiddlewareFromFunc(func(w http.ResponseWriter, r *http.Request, next server.HandlerFunc) error {
		claims, err := svc.Verify(r)
		if err != nil {
			return server.NewHTTPError(http.StatusUnauthorized, "unauthorized")
		}
		return next(w, authKey.WithRequest(r, claims))
	})
}

func handler(ctx context.Context, in *Input) (*Output, error) {
	claims, ok := authKey.From(ctx)
	// ...
}
```

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
