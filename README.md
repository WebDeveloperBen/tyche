# tyche

Typed Go HTTP handlers that generate their own OpenAPI spec — running over
**any router you bring**, with optional zero-reflection request/response codecs.

You write a handler as `func(ctx, *In) (*Out, error)`. tyche derives the OpenAPI
operation from the types and binds/validates/serializes via reflection, so it
runs with no build step. Optionally, `tyche generate` emits that binding,
validation, and serialization code ahead of time for a reflection-free fast path
(like `sqlc`, but for HTTP I/O). Routing is delegated to a pluggable `Adapter` —
the standard library `net/http.ServeMux` by default, or chi/gin/anything you
wire up.

## Design in one paragraph

tyche owns the parts that benefit from being typed and generated — request
binding, validation, response serialization, and the OpenAPI document — and
deliberately does **not** own routing. Path matching is a solved problem with
several fast, battle-tested implementations, so tyche exposes a small `Adapter`
interface and lets you choose. The core module depends only on the standard
library; third-party routers are opt-in glue you supply.

## Install

There are two installs, depending on which side of the API you're on.

**Using tyche in your own project** (you write a server, or generate a client
from one). You need both — the Go packages for the imports to resolve, the CLI
binary to drive `init` / `generate` / `client` / `test` / `build` / `run`.

```sh
# 1. Add tyche to your go.mod:
go get github.com/webdeveloperben/tyche

# 2. Install the tyche CLI once. It lives on $GOPATH/bin and works from
#    any project directory:
go install github.com/webdeveloperben/tyche/cmd/tyche@latest

# 3. In your project, scaffold the config (one file, committed to git):
cd myproject
tyche init --module github.com/me/myproject/client --yes
tyche generate            # emits server codecs, if you have any
tyche client              # regenerates the typed client from spec
```

The CLI is a single binary; you do **not** add it to your project's go.mod. The
libraries (`server`, `server/apidocs`, `server/plugins`, `clientgen`, etc.) are
regular Go packages you import and version through your go.mod.

**Prefer a prebuilt binary?** Every release ships static binaries for macOS,
Linux, and Windows (amd64/arm64) on the
[Releases page](https://github.com/webdeveloperben/tyche/releases) — no Go
toolchain required. Download the archive for your platform, verify it against
`checksums.txt`, extract `tyche`, and drop it on your `PATH`:

```sh
# Example: macOS arm64. Swap in the version + platform you need.
VER=1.2.3
curl -fsSLO "https://github.com/webdeveloperben/tyche/releases/download/v${VER}/tyche_${VER}_darwin_arm64.tar.gz"
tar -xzf "tyche_${VER}_darwin_arm64.tar.gz"
sudo mv tyche /usr/local/bin/
tyche version
```

**Working on tyche itself.** Clone, install the toolchain, run the task suite:

```sh
git clone https://github.com/webdeveloperben/tyche
cd tyche
mise install              # installs the pinned Go version
lefthook install          # sets up the pre-commit hook
task tests                # full suite in a generated worktree
go build -o ./bin/tyche ./cmd/tyche
./bin/tyche --help
task verify:cli           # smoke-test the CLI surface
```

Cross-router benchmarks live in `benchmarks/comparison/` as their own
sub-module so chi/gin/huma don't leak into your tyche build. Run them
with `cd benchmarks/comparison && go test -bench=.`.

See [CONTRIBUTING.md](CONTRIBUTING.md) for the day-to-day dev loop, the test
shapes, and the rules for breaking changes.

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

With tyche installed (see the section above), this runs as-is — `go run .` is
enough. Typed routes bind, validate, and serialize via reflection out of the box.
Running `tyche generate` emits zero-reflection codecs that replace the reflection
path for a speed-up; it is an optimization, not a requirement:

```sh
tyche generate ./...   # optional: emit zero-reflection codecs
```

### Build your own API

The Quick Start above is the whole shape; this is the literal end-to-end flow
from a fresh directory to a running server with generated codecs.

```sh
# 1. Create a project.
mkdir myapi && cd myapi
go mod init github.com/me/myapi

# 2. Add tyche.
go get github.com/webdeveloperben/tyche
go install github.com/webdeveloperben/tyche/cmd/tyche@latest

# 3. Drop the Quick Start code into main.go (or any package).

# 4. Scaffold the config.
tyche init --module github.com/me/myapi/client --yes

# 5. Generate codecs (writes zz_server_routes_gen.go next to main.go).
tyche generate

# 6. Run.
go run .
# curl localhost:8080/api/users/u1
```

The generated `zz_server_routes_gen.go` next to `main.go` is registered in an
`init()` and replaces the reflection path for the route defined in
`server.Register`. Both paths produce byte-identical responses, so the
generated file is an optimization, not a behaviour change. Delete it and the
reflection path takes over with no other edits.

If you want to skip the reflection path entirely, build with a flag:

```sh
tyche build -o ./bin/api .        # generate, then go build .
tyche run .                       # generate, then go run .
tyche test ./...                  # generate, then go test ./...
```

These all run `go run`/`go build`/`go test` against a temporary copy of your
project with codecs generated in place, so the real working tree is never
touched by generated code.

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
lives in `benchmarks/comparison/adapter_chi_test.go` (a separate sub-module
that pulls in chi only for the comparison benchmarks) and can be copied
into your project as-is.

## Typed routes and generated codecs

Register a handler as `func(ctx, *In) (*Out, error)`. Input fields are bound from
`path`, `query`, `header`, and `cookie` tags plus either a JSON body (`Body`
field, `body` tag, or JSON-tagged fields) or a multipart form body (`form`,
`file`, and `files` tags). Output fields map to a JSON body, response headers,
and status.

By default this runs through a reflection binder — no codegen step, so iterating
with `go run` is friction-free. `tyche generate` then inspects each
`server.Register(...)` call and emits a codec (`zz_server_routes_gen.go`) that
does the binding, validation, and serialization with hand-written byte-level
code and **no runtime reflection** — conceptually the same trade `sqlc` makes for
SQL. The generated codec is registered in an `init()` and picked up
automatically; when present it replaces the reflection path for that route. Both
paths produce byte-identical responses, so codegen is a pure performance
optimization you reach for in production, not a prerequisite.

Multipart routes are supported by both the reflection binder and generated
server codecs. Generated multipart codecs use the same form/file semantics as
the runtime binder.

Successful responses are wrapped in a `{"data": …}` envelope; errors are RFC
9457 `application/problem+json`. JSON request bodies reject unsupported
`Content-Type` values with 415, and JSON/SSE responses return 406 when the
request `Accept` header does not allow the produced media type. Generated route
metadata records the response content types emitted by generated codecs. The
server-side `Codec` interface now owns the JSON request decode and success
envelope path, with `JSONCodec` as the default implementation. Additional
server-wide codecs can be registered on `APIConfig.Codecs`; JSON remains
registered by default, and typed route OpenAPI content maps include configured
codec media types for JSON request and success bodies.

Route input/output types may live anywhere, including a single-file `package
main` — tyche keys main-package codecs to match Go's runtime reflection, so
the small everything-in-`main.go` app works end to end.

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

## Content negotiation

Typed routes use `application/json` by default. Register extra server codecs
with `APIConfig.Codecs`; `JSONCodec` remains available automatically. OpenAPI
advertises every configured codec for non-multipart request bodies and success
responses, and runtime request/response selection uses `Content-Type` and
`Accept`.

```go
api := server.NewAPI(adapter, server.APIConfig{
	Codecs: []server.Codec{myVendorJSONCodec},
})

server.Register(api, op, handler,
	server.WithRequestContentTypes("application/json"),
	server.WithResponseContentTypes("application/vnd.example+json"),
)
```

When extra codecs are configured, older JSON-only generated route codecs are
bypassed in favour of the negotiated reflection path. Regenerated route codecs
receive the route codec set and keep the fast JSON path when JSON is selected.

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
go install github.com/webdeveloperben/tyche/cmd/tyche@latest

tyche init                               # scaffold tyche.json next to go.mod
tyche config show                        # show the resolved config

tyche generate ./...                     # emit server codecs
tyche build -o ./bin/api ./cmd/api       # generate, then build
tyche run ./cmd/api                      # generate, then run
tyche test ./...                         # generate, then test

tyche client                             # regenerate the typed client from spec
# optional: keep distinct output/body/event types per operation
tyche client --type-naming operation-scoped

tyche version                            # print build identity
tyche completion bash > /etc/bash_completion.d/tyche
```

A `tyche.json` at the project root holds the inputs the CLI would otherwise take
as flags. Discovery walks up from cwd to the first `go.mod`. Flags always
override file values. Pass `--config <path>` to point at a non-default file or
`--quiet` to suppress the "using config ..." line. Set `TYCHE_CONFIG` to point
the CLI at a specific file via the environment.

Every command honours a global `--format` flag (`human|json|quiet`). Use
`--format=json` to get machine-readable output suitable for `| jq`; use
`--format=quiet` in CI and scripts where you only want the data line.

### Embedding the CLI use-cases

The CLI is split so the use-case logic (scaffold, generate, regenerate
client, worktree plumbing) lives in `internal/app` and is reachable
without the CLI. If you embed tyche in your own tooling:

```go
import "github.com/webdeveloperben/tyche/internal/app"

written, err := app.Scaffold(app.ScaffoldOptions{
    Root:   "/path/to/project",
    Module: "github.com/acme/api/client",
    Force:  false,
})
```

`internal/app` takes plain Go values; the CLI is a thin Kong adapter on
top. The servergen, clientgen, and server packages never import
`internal/cli` or any CLI framework.

## Generated Go client

`tyche client` generates a self-contained, standard-library-only typed Go
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
(scanner API: `Next`/`Event`/`EventName`/`ID`/`Retry`/`Err`/`Close`).

String and integer enums generate a named type with typed constants, `allOf`
compositions of objects are merged into a single struct (keeping the component
name), and a success response in a non-JSON media type returns `[]byte` rather
than being decoded or dropped.

Operations with `multipart/form-data` request bodies generate `form`, `file`,
and `files` input fields. File inputs use the generated `client.File` type,
which carries the part filename, content reader, and optional content type.
Non-multipart request/response encoding goes through the generated `Codec`
interface; `client.WithCodec(...)` can swap the default `JSONCodec` for a
compatible JSON vendor media type or another implementation. The codec owns
its own `MediaType()` and `MatchesResponse(...)`, so a vendor JSON codec
that wants to accept plain `application/json` responses (or vice versa)
encodes that decision in the codec rather than in the runtime. By default,
raw downloads and other non-envelope success responses send `Accept` from
each operation's documented success media types, so a `/report` operation
returns `[]byte` against `Accept: application/pdf` instead of being decoded
through the codec.

By default, structurally identical schemas share one generated Go type. Use
`--type-naming operation-scoped` when distinct operations should keep distinct
body/output/event types even if their schemas have the same shape.

**Current limitations:** `oneOf`/`anyOf` unions plus non-object `allOf`
compositions are emitted as `json.RawMessage`.

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

## Developing tyche

For day-to-day contributor workflow, test shapes, and the rules for breaking
changes, see [CONTRIBUTING.md](CONTRIBUTING.md). The short version:

- Clone, `mise install`, `lefthook install`.
- `task tests` runs the full suite through a generated worktree (the way CI does
  it). `go test ./...` runs the same tests directly without the worktree.
- `go build -o ./bin/tyche ./cmd/tyche` then `./bin/tyche --help` exercises the
  CLI locally without installing.
- Lint with `golangci-lint run ./...`; check modernization with
  `task modernize:check`; fix with `task modernize`.
- Breaking changes are allowed but must be called out in the PR description
  and added to the "Unreleased" section at the top of [CHANGELOG.md](CHANGELOG.md).
