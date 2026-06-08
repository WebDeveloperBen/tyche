# tyche

Typed Go HTTP routing with OpenAPI generation, pluggable docs UIs, and optional generated request/response codecs.

## What it gives you

- Fast router with static, param, and wildcard paths.
- Typed route registration with OpenAPI output.
- Generated codecs via `servergen` to avoid reflection on supported routes.
- Built-in docs mounting for Scalar and Redoc.
- Middleware at root, group, and route scope, plus `http.Handler` middleware.
- Typed Server-Sent Events streaming (`RegisterStream`) with OpenAPI output.
- Pluggable error handling, custom 404/405, per-route body limits, and `Mount` for sub-handlers.
- OpenAPI security schemes and per-operation security requirements.
- Dependency-free request instrumentation hooks and a `servertest` helper package.

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

## Error handling

By default the router renders `HTTPError` and validation errors as RFC 9457
`application/problem+json`, and unmatched routes / methods return problem+json
404 / 405. Override any of these:

```go
router := server.NewRouterWithConfig(server.RouterConfig{
	ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
		// e.g. map upstream provider errors, attach request IDs, log 5xx.
		server.DefaultErrorHandler(w, r, err)
	},
})

router.SetNotFoundHandler(myNotFound)          // http.Handler
router.SetMethodNotAllowedHandler(my405)       // http.Handler
```

## Streaming (Server-Sent Events)

`RegisterStream` registers a typed SSE endpoint: input is bound and validated
like any typed route, the response is documented in OpenAPI as
`text/event-stream`, and the handler streams type-safe events. Root/group/route
middleware all apply.

```go
type StreamInput struct {
	Topic string `query:"topic" required:"true"`
}
type Token struct {
	Text string `json:"text"`
}

server.RegisterStream(api, server.Operation{
	OperationID: "stream-tokens",
	Method:      http.MethodGet,
	Path:        "/chat/stream",
}, func(ctx context.Context, in *StreamInput, stream *server.Stream[Token]) error {
	for tok := range tokens(ctx, in.Topic) {
		if err := stream.Send(Token{Text: tok}); err != nil {
			return err // client disconnected
		}
	}
	return nil
})
```

For non-typed handlers, `server.NewEventStream(w, r)` returns a raw `EventStream`
with `Send`, `SendData`, `Comment`, and `Flush`.

## OpenAPI security

Register schemes once and reference them per operation:

```go
router.AddSecurityScheme("bearerAuth", server.BearerScheme("JWT"))
router.AddSecurityScheme("apiKey", server.APIKeyScheme("X-API-Key", "header"))

server.Register(api, server.Operation{
	OperationID: "create-thing",
	Method:      http.MethodPost,
	Path:        "/things",
	Security:    []server.SecurityRequirement{{"bearerAuth": {}}},
}, handler)
```

## Per-route body limits and mounting

```go
// Override the router-wide MaxRequestBodyBytes for one route (0 = unlimited).
server.Register(api, uploadOp, uploadHandler, server.WithMaxBodyBytes(100<<20))

// Mount any http.Handler (pprof, a metrics endpoint, another mux) at a prefix.
router.Mount("/debug/pprof", pprofMux)
```

## Instrumentation

A dependency-free seam for tracing/metrics, reporting method, route template,
status, response bytes, and duration per request. Two variants:

- **`plugins.InstrumentHTTP`** (recommended for metrics) — `UseHTTP` middleware
  that wraps the whole router, so `Status`/`Bytes` reflect the **final** response,
  including error and 404/405 responses.
- **`plugins.Instrument`** — `HandlerFunc` middleware that additionally exposes
  the handler's returned Go `error` (`RequestInfo.Err`), but its `Status` only
  reflects what the handler itself wrote (errors are rendered by the router
  afterwards, so they show as 200 here — classify them via `Err`).

```go
router.UseHTTP(plugins.InstrumentHTTP(plugins.ObserverFunc(func(i plugins.RequestInfo) {
	histogram.WithLabelValues(i.Method, i.Route, strconv.Itoa(i.Status)).Observe(i.Duration.Seconds())
})))
```

`server.RoutePattern(r)` exposes the matched route template (e.g. `/users/:id`)
to your own middleware and handlers — use it as a low-cardinality metric label.

## Testing

The `servertest` package builds requests and unwraps the standard `DataResponse`
envelope:

```go
client := servertest.New(t, router)
resp := client.POST("/users", User{Name: "Ada"}).AssertStatus(http.StatusCreated)
got := servertest.DecodeData[User](t, resp)

problem := servertest.DecodeProblem(t, client.GET("/secret").AssertStatus(401))
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
servergen client --spec openapi.json --out ./client --module github.com/you/app/client
```

Optional staging excludes can go in `.servergenignore`.

## Generated Go client

`servergen client` generates a **self-contained, dependency-free** typed Go
client from the OpenAPI spec your server emits. It's for Go code that *consumes*
your API — other services, a CLI, or a customer SDK — giving typed calls instead
of hand-rolled HTTP. (Non-Go consumers should generate from the OpenAPI spec
with their own language's tooling.)

```sh
# in your API repo: emit the spec, then generate + commit the client
servergen client --spec openapi.json --out ./client \
  --module github.com/you/app/client
```

The output module imports only the standard library and bakes in tyche's
conventions — the `{"data": …}` success envelope and `application/problem+json`
errors (as a typed `*APIError`):

```go
import "github.com/you/app/client"

c := client.New("https://api.you.com", client.WithBearerToken(tok))

out, err := c.GetUser(ctx, &client.GetUserInput{ID: "u1"})
var apiErr *client.APIError
if errors.As(err, &apiErr) && apiErr.StatusCode == 404 { /* ... */ }
```

Every method takes trailing `...client.CallOption` for per-call extras — add a
request header, or read response headers/status:

```go
var resp *http.Response
out, err := c.GetUser(ctx, in,
	client.WithRequestHeader("Idempotency-Key", key),
	client.WithResponseCallback(func(r *http.Response) { resp = r }), // read r.Header
)
```

Because the client is its own module with its own version tags, consumers
`go get …/client@vX.Y.Z` and the contract is checked at compile time. Types are
recovered from the spec by structural deduplication, so a shape returned by
several endpoints collapses to one Go type.

Server-Sent Events operations (registered server-side with `RegisterStream`)
generate a streaming method returning a typed `*Stream[Event]`, iterated with the
scanner pattern:

```go
s, err := c.StreamMessages(ctx, &client.StreamMessagesInput{Topic: "general"})
if err != nil { /* ... */ }
defer s.Close()
for s.Next() {
	ev := s.Event() // typed event value
	// ...
}
if err := s.Err(); err != nil { /* ... */ }
```

`clientgen.Generate` is the programmatic entry point if you'd rather generate
in-process from a `*clientgen.Document`.

**Current limitations:** structural dedup means two distinct shapes with
identical structure share one Go type (named from the first occurrence);
integer enums generate a bare integer type (no named constants); schema
composition (`allOf`/`oneOf`/`anyOf`) is emitted as `json.RawMessage` (with a
generation notice); and successful responses are decoded as JSON regardless of
`Content-Type`.

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
