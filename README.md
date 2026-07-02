# tyche

Typed Go HTTP handlers that generate their own OpenAPI spec and zero-reflection
request/response codecs — running over **any router you bring**.

You write a handler as `func(ctx, *In) (*Out, error)`. tyche derives the OpenAPI
operation from the types, and `servergen` generates the binding, validation, and
serialization code ahead of time (like `sqlc`, but for HTTP I/O). Routing itself
is delegated to a pluggable `Adapter` — the standard library `net/http.ServeMux`
by default, or chi/gin/anything you wire up.

## Design in one paragraph

tyche owns the parts that benefit from being typed and generated — request
binding, validation, response serialization, and the OpenAPI document — and
deliberately does **not** own routing. Path matching is a solved problem with
several fast, battle-tested implementations, so tyche exposes a small `Adapter`
interface and lets you choose. The core module depends only on the standard
library; third-party routers are opt-in glue you supply.

## Quick start

```go
package main

import (
	"context"
	"net/http"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/apidocs"
)

type GetUserInput struct {
	ID string `path:"id"`
}

type GetUserOutput struct {
	Body struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}
}

func getUser(ctx context.Context, in *GetUserInput) (*GetUserOutput, error) {
	out := &GetUserOutput{}
	out.Body.ID, out.Body.Name = in.ID, "Ada"
	return out, nil
}

func main() {
	// The stdlib ServeMux adapter is the zero-dependency default.
	api := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		OpenAPI:             server.OpenAPIInfo{Title: "Example API", Version: "1.0.0"},
		MaxRequestBodyBytes: 10 << 20,
	})

	v1 := api.Group("/api")

	server.Register(v1, server.Operation{
		OperationID: "get-user",
		Method:      http.MethodGet,
		Path:        "/users/:id",
	}, getUser)

	_ = apidocs.Mount(api, apidocs.Config{
		SpecPath: "/openapi.json",
		UIs: []apidocs.UIMount{
			{Path: "/docs", Renderer: apidocs.Scalar()},
		},
	})

	_ = http.ListenAndServe(":8080", api)
}
```

Typed routes require generated codecs. Run the generator (or use the `servergen`
run/build wrappers, which regenerate first):

```sh
servergen generate ./...
```

## The adapter model

An `Adapter` maps a `(method, path)` to a handler and dispatches requests. That
is the only thing tyche delegates; binding, validation, serialization,
middleware, error rendering, and OpenAPI are layered on top and are identical
regardless of adapter.

```go
type Adapter interface {
	Handle(method, path string, h http.Handler)
	ServeHTTP(w http.ResponseWriter, r *http.Request)
	SetFallback(notFound, methodNotAllowed http.Handler)
}
```

Paths arrive in tyche's template form (`:name` params, `*name` trailing
wildcard); each adapter translates to its router's syntax.

### Standard library (default)

`server.NewServeMuxAdapter()` uses `net/http.ServeMux`. Because tyche's binders
read path parameters via `(*http.Request).PathValue`, the stdlib adapter is a
zero-glue fit — ServeMux's native `{name}` matching populates exactly what the
codecs read.

### Bring your own router

Any router works in ~40 lines. The one router-specific detail is bridging its
matched params onto `req.PathValue` so the binding layer stays agnostic. A chi
adapter, in full:

```go
type ChiAdapter struct{ mux chi.Router }

func NewChiAdapter() *ChiAdapter { return &ChiAdapter{mux: chi.NewRouter()} }

func (a *ChiAdapter) Handle(method, path string, h http.Handler) {
	bridged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			for i, key := range rctx.URLParams.Keys {
				r.SetPathValue(key, rctx.URLParams.Values[i])
			}
		}
		h.ServeHTTP(w, r)
	})
	a.mux.Method(method, toChiPattern(path), bridged) // ":id" -> "{id}", "*" -> "*"
}

func (a *ChiAdapter) SetFallback(notFound, methodNotAllowed http.Handler) {
	a.mux.NotFound(notFound.ServeHTTP)
	a.mux.MethodNotAllowed(methodNotAllowed.ServeHTTP)
}

func (a *ChiAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }
```

tyche does **not** ship chi/gin/fiber adapters — that would bind the module to
those routers' versions. The interface is the contract; a reference chi adapter
lives in `server/adapter_spike_test.go` to copy from.

## Typed routes and generated codecs

Register a handler as `func(ctx, *In) (*Out, error)`. Input fields are bound from
`path`, `query`, `header`, and `cookie` tags plus a JSON body (`Body` field or a
`body` tag); output fields map to a JSON body, response headers, and status.

`servergen` inspects each `server.Register(...)` call at build time and emits a
codec (`zz_server_routes_gen.go`) that does the binding, validation, and
serialization with hand-written byte-level code and no runtime reflection —
conceptually the same trade `sqlc` makes for SQL. The generated codec is
registered in an `init()` and picked up automatically at registration.

Successful responses are wrapped in a `{"data": …}` envelope; errors are RFC
9457 `application/problem+json`.

## Middleware

Middleware is `func(next HandlerFunc) HandlerFunc`, applied at three scopes that
run outermost to innermost: root, group, then route.

```go
// Root-level middleware, applied to every route (outermost first):
api.Use(plugins.Recoverer(), plugins.RealIP())

// Group-level tyche middleware:
v1 := api.Group("/v1", middleware.Auth(authSvc), middleware.AccessLog(logger))

// Route-level middleware, immediately before the handler:
server.Register(v1, chatOp, chatHandler,
	server.WithMiddleware(middleware.RequireScope("llm:chat")),
)
```

Helpers: `server.MiddlewareFromFunc(fn)`, `server.Chain(mw...)`,
`server.WithMiddleware(mw...)`, `server.NamedMiddleware` + `UseNamed(...)`, and
`server.NewContextKey[T](name)` for typed request-scoped values.

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
```

## Error handling

By default `HTTPError` and validation errors render as RFC 9457
`application/problem+json`, and unmatched routes / methods return problem+json
404 / 405 — on every adapter. Override any of these:

```go
api.SetErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
	// map upstream provider errors, attach request IDs, log 5xx, then:
	server.DefaultErrorHandler(w, r, err)
})
api.SetNotFoundHandler(myNotFound)         // http.Handler
api.SetMethodNotAllowedHandler(my405)      // http.Handler
```

## Streaming (Server-Sent Events)

`RegisterStream` registers a typed SSE endpoint: input is bound and validated
like any typed route, the response is documented in OpenAPI as
`text/event-stream`, and the handler streams type-safe events. (Streaming binds
via reflection, since there is no generated codec for a streamed response.)

```go
type StreamInput struct {
	Topic string `query:"topic" required:"true"`
}
type Token struct {
	Text string `json:"text"`
}

server.RegisterStream(v1, server.Operation{
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

For non-typed handlers, `server.NewEventStream(w, r)` returns a raw `EventStream`.

## OpenAPI security

```go
api.AddSecurityScheme("bearerAuth", server.BearerScheme("JWT"))
api.AddSecurityScheme("apiKey", server.APIKeyScheme("X-API-Key", "header"))

server.Register(v1, server.Operation{
	OperationID: "create-thing",
	Method:      http.MethodPost,
	Path:        "/things",
	Security:    []server.SecurityRequirement{{"bearerAuth": {}}},
}, handler)
```

## Per-route body limits and mounting

```go
// Override the API-wide MaxRequestBodyBytes for one route (0 = unlimited).
server.Register(v1, uploadOp, uploadHandler, server.WithMaxBodyBytes(100<<20))

// Mount any http.Handler (pprof, a metrics endpoint, another mux) at a prefix.
api.Mount("/debug/pprof", pprofMux)
```

## Instrumentation

A dependency-free seam for tracing/metrics reporting method, route template,
status, response bytes, and duration per request:

```go
api.UseHTTP(plugins.InstrumentHTTP(plugins.ObserverFunc(func(i plugins.RequestInfo) {
	histogram.WithLabelValues(i.Method, i.Route, strconv.Itoa(i.Status)).Observe(i.Duration.Seconds())
})))
```

`server.RoutePattern(r)` returns the matched route template (e.g. `/users/:id`)
as a low-cardinality metric label. OpenTelemetry is intentionally not a hard
dependency; this is the bridge seam.

## Testing

The `servertest` package builds requests against any `http.Handler` (an `*API`)
and unwraps the standard `DataResponse` envelope:

```go
client := servertest.New(t, api)
resp := client.POST("/users", User{Name: "Ada"}).AssertStatus(http.StatusCreated)
got := servertest.DecodeData[User](t, resp)

problem := servertest.DecodeProblem(t, client.GET("/secret").AssertStatus(401))
```

## CLI

```sh
go install ./cmd/servergen

servergen generate ./...                # emit codecs
servergen build -o ./bin/api ./cmd/api  # generate, then build
servergen run ./cmd/api                 # generate, then run
servergen test ./...                    # generate, then test
servergen client --spec openapi.json --out ./client --module github.com/you/app/client
```

Optional staging excludes go in `.servergenignore`.

## Generated Go client

`servergen client` generates a self-contained, standard-library-only typed Go
client from the OpenAPI spec your server emits — for Go code that *consumes* your
API (other services, a CLI, a customer SDK). It bakes in tyche's conventions: the
`{"data": …}` envelope and problem+json errors as a typed `*APIError`.

```go
c := client.New("https://api.you.com", client.WithBearerToken(tok))

out, err := c.GetUser(ctx, &client.GetUserInput{ID: "u1"})
var apiErr *client.APIError
if errors.As(err, &apiErr) && apiErr.StatusCode == 404 { /* ... */ }
```

Because the client is its own module with its own tags, consumers
`go get …/client@vX.Y.Z` and the contract is checked at compile time. SSE
operations generate a streaming method returning a typed `*Stream[Event]`
(scanner API: `Next`/`Event`/`Err`/`Close`).

**Current limitations:** structural dedup means two distinct shapes with
identical structure share one Go type; integer enums generate a bare integer
type; `allOf`/`oneOf`/`anyOf` are emitted as `json.RawMessage`; and successful
responses are decoded as JSON regardless of `Content-Type`.

## Benchmarks

Measured on Apple M3 Pro, tyche over the stdlib `ServeMuxAdapter`:

```sh
task benchmark:comparison
```

| Benchmark | tyche (stdlib) | chi | gin | huma |
| --- | ---: | ---: | ---: | ---: |
| Static route | 453 ns · 3 allocs | 320 · 2 | 309 · 2 | 992 · 8 |
| Param route | 482 ns · 4 allocs | 314 · 5 | 296 · 3 | 1076 · 10 |
| **Body (bind+validate+serialize)** | **1370 ns · 13 allocs** | 2334 · 19 | 2435 · 19 | 4708 · 41 |
| **Nested body** | **1780 ns · 22 allocs** | 1870 · 23 | 1931 · 23 | 4269 · 48 |

How to read this:

- **Routing cost is your adapter's cost.** On trivial static/param routes the
  stdlib-backed path carries a little overhead (ServeMux's own match allocations
  plus the edge tracked-writer) versus a raw router. If routing is your
  bottleneck, plug in gin or chi via the adapter and keep everything else.
- **The generated codec is where tyche pays off.** On real endpoints that bind,
  validate, and serialize a JSON body, tyche is ~1.7× faster than chi/gin and
  ~3× faster than huma — and that advantage is adapter-independent, because the
  codec is the same no matter what routes the request.

Treat these as regression baselines, not a definitive shootout.
