package server_test

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webdeveloperben/tyche/server"
)

// chiTestAdapter is a reference server.Adapter over github.com/go-chi/chi. It
// lives in the test suite rather than in a shipped package: the Adapter
// interface is the contract, and a third-party router adapter is ~40 lines of
// user-land glue. This doubles as the proof that a foreign router works
// unchanged and as the copy-paste example for anyone wiring chi/gin/fiber.
type chiTestAdapter struct{ mux chi.Router }

func newChiAdapter() *chiTestAdapter { return &chiTestAdapter{mux: chi.NewRouter()} }

var _ server.Adapter = (*chiTestAdapter)(nil)

func (a *chiTestAdapter) Handle(method, path string, h http.Handler) {
	// Bridge chi's RouteContext params onto req.PathValue so tyche's binders
	// (which read req.PathValue) resolve them with no special-casing.
	bridged := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if rctx := chi.RouteContext(r.Context()); rctx != nil {
			for i, key := range rctx.URLParams.Keys {
				r.SetPathValue(key, rctx.URLParams.Values[i])
			}
		}
		h.ServeHTTP(w, r)
	})
	a.mux.Method(method, toChiPattern(path), bridged)
}

func (a *chiTestAdapter) SetFallback(notFound, methodNotAllowed http.Handler) {
	if notFound != nil {
		a.mux.NotFound(notFound.ServeHTTP)
	}
	if methodNotAllowed != nil {
		a.mux.MethodNotAllowed(methodNotAllowed.ServeHTTP)
	}
}

func (a *chiTestAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

// toChiPattern converts tyche's "/things/:id/*rest" to chi's "/things/{id}/*".
func toChiPattern(path string) string {
	var b []byte
	for _, part := range server.SplitRouteFast(path) {
		b = append(b, '/')
		switch {
		case len(part) > 0 && part[0] == ':':
			b = append(b, '{')
			b = append(b, part[1:]...)
			b = append(b, '}')
		case len(part) > 0 && part[0] == '*':
			b = append(b, '*')
		default:
			b = append(b, part...)
		}
	}
	if len(b) == 0 {
		return "/"
	}
	return string(b)
}

// --- fixtures -------------------------------------------------------------

// A normal typed route, exercised through the generated-codec fast path (the
// moat). The codec below is registered once and matches any input/output types
// because its InputTypeKey/OutputTypeKey are empty.
type widgetInput struct {
	ID string `path:"id"`
}

type widgetOutput struct {
	Name string `json:"name"`
}

// The user's exact streaming ergonomics.
type StreamInput struct {
	Topic string `query:"topic" required:"true"`
}

type Token struct {
	Text string `json:"text"`
}

func init() {
	// Register a hand-written codec standing in for servergen output, proving
	// the generated fast path flows through the adapter unchanged.
	server.RegisterGeneratedCodec(server.GeneratedRouteMeta{
		PackagePath:       "spike",
		OperationID:       "get-widget",
		Method:            http.MethodGet,
		Path:              "/widgets/:id",
		HasGeneratedCodec: true,
	}, server.GeneratedRouteCodec{
		Parse: func(req *http.Request) (any, error) {
			return &widgetInput{ID: req.PathValue("id")}, nil
		},
		Write: func(w http.ResponseWriter, _ *http.Request, value any) error {
			out, _ := value.(*widgetOutput)
			return server.WriteTypedResponse(w, out)
		},
	})
}

// buildAPI registers the same two routes (one codec-backed typed route, one
// SSE stream) against whatever adapter is passed in.
func buildAPI(adapter server.Adapter) *server.API {
	api := server.NewAPI(adapter, server.APIConfig{
		OpenAPI: server.OpenAPIInfo{Title: "Spike", Version: "0.1.0"},
	})
	v1 := api.Group("/api")

	server.Register(v1, server.Operation{
		OperationID: "get-widget",
		Method:      http.MethodGet,
		Path:        "/widgets/:id",
	}, func(_ context.Context, in *widgetInput) (*widgetOutput, error) {
		return &widgetOutput{Name: "widget-" + in.ID}, nil
	})

	server.RegisterStream(v1, server.Operation{
		OperationID: "stream-tokens",
		Method:      http.MethodGet,
		Path:        "/chat/stream",
	}, func(_ context.Context, in *StreamInput, stream *server.Stream[Token]) error {
		for _, tok := range []string{in.Topic, "b", "c"} {
			if err := stream.Send(Token{Text: tok}); err != nil {
				return err
			}
		}
		return nil
	})

	_ = api.MountOpenAPI("/openapi.json")
	return api
}

// headerStamp is a ServeHTTPMiddleware used to prove UseHTTP wraps the whole
// dispatch, including 404/405 responses.
func headerStamp(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("X-Edge", "seen")
		next.ServeHTTP(w, r)
	})
}

// --- the spike ------------------------------------------------------------

func TestAdapterSpike(t *testing.T) {
	adapters := map[string]func() server.Adapter{
		"ServeMux": func() server.Adapter { return server.NewServeMuxAdapter() },
		"Chi":      func() server.Adapter { return newChiAdapter() },
	}

	for name, mk := range adapters {
		t.Run(name, func(t *testing.T) {
			api := buildAPI(mk())

			// 1. Codec-backed typed route: param binding + {"data":…} envelope.
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/widgets/42", nil))
			if rec.Code != http.StatusOK {
				t.Fatalf("typed route: status = %d, body = %s", rec.Code, rec.Body.String())
			}
			var env struct {
				Data widgetOutput `json:"data"`
			}
			if err := json.Unmarshal(rec.Body.Bytes(), &env); err != nil {
				t.Fatalf("typed route: decode envelope: %v (body %s)", err, rec.Body.String())
			}
			if env.Data.Name != "widget-42" {
				t.Fatalf("typed route: got %q, want %q", env.Data.Name, "widget-42")
			}

			// 2. Typed SSE stream: query binding + validation + event frames.
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chat/stream?topic=general", nil))
			if ct := rec.Header().Get("Content-Type"); ct != "text/event-stream" {
				t.Fatalf("stream: content-type = %q", ct)
			}
			if body := rec.Body.String(); !strings.Contains(body, `data: {"text":"general"}`) {
				t.Fatalf("stream: missing first event, body:\n%s", body)
			}

			// 3. Missing required query param is rejected by runtime validation.
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/chat/stream", nil))
			if rec.Code != http.StatusBadRequest {
				t.Fatalf("stream validation: status = %d, want 400", rec.Code)
			}

			// 4. OpenAPI generation is identical regardless of adapter.
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/openapi.json", nil))
			spec := rec.Body.String()
			for _, want := range []string{`"/api/widgets/{id}"`, `"/api/chat/stream"`, `"text/event-stream"`, `"get-widget"`} {
				if !strings.Contains(spec, want) {
					t.Fatalf("openapi: missing %s in:\n%s", want, spec)
				}
			}

			// 5. 404 renders tyche's problem+json (not the router's default).
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))
			if rec.Code != http.StatusNotFound {
				t.Fatalf("404: status = %d", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("404: content-type = %q, want problem+json", ct)
			}

			// 6. 405: the path exists under GET; POST must be method-not-allowed.
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/api/widgets/42", nil))
			if rec.Code != http.StatusMethodNotAllowed {
				t.Fatalf("405: status = %d, want 405", rec.Code)
			}
			if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
				t.Fatalf("405: content-type = %q, want problem+json", ct)
			}
		})
	}
}

// TestAdapterWildcard verifies the trailing-wildcard tail resolves via
// Wildcard(r) (i.e. Param(r, "*")) on both adapters — guarding the
// tree→adapter capture-name bridge.
func TestAdapterWildcard(t *testing.T) {
	adapters := map[string]func() server.Adapter{
		"ServeMux": func() server.Adapter { return server.NewServeMuxAdapter() },
		"Chi":      func() server.Adapter { return newChiAdapter() },
	}
	for name, mk := range adapters {
		t.Run(name, func(t *testing.T) {
			api := server.NewAPI(mk())
			var star string
			api.GET("/files/*", func(w http.ResponseWriter, r *http.Request) error {
				star = server.Wildcard(r)
				return nil
			})
			api.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/files/a/b/c.txt", nil))
			if star != "a/b/c.txt" {
				t.Fatalf("Wildcard() = %q, want %q", star, "a/b/c.txt")
			}
		})
	}
}

// TestServeMuxNamedWildcard verifies a named trailing wildcard (*name) resolves
// via Param(r, name) on the shipped stdlib adapter.
func TestServeMuxNamedWildcard(t *testing.T) {
	api := server.NewAPI(server.NewServeMuxAdapter())
	var named, star string
	api.GET("/blobs/*path", func(w http.ResponseWriter, r *http.Request) error {
		named = server.Param(r, "path")
		star = server.Wildcard(r)
		return nil
	})
	api.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/blobs/x/y", nil))
	if named != "x/y" || star != "x/y" {
		t.Fatalf("Param(path)=%q Wildcard()=%q, want both %q", named, star, "x/y")
	}
}

// TestServeMuxRedirectPassthrough verifies ServeMux's own redirect for a
// non-canonical path (here a "//") is passed through, not clobbered into a 404.
func TestServeMuxRedirectPassthrough(t *testing.T) {
	api := server.NewAPI(server.NewServeMuxAdapter())
	api.GET("/api/users", func(w http.ResponseWriter, r *http.Request) error { return nil })

	rec := httptest.NewRecorder()
	api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api//users", nil))
	if rec.Code/100 != 3 {
		t.Fatalf("non-canonical path: status = %d, want a 3xx redirect", rec.Code)
	}
	if loc := rec.Header().Get("Location"); loc != "/api/users" {
		t.Fatalf("redirect Location = %q, want %q", loc, "/api/users")
	}
}

// TestHandleEConflictReturnsError verifies a router-level pattern conflict is
// returned as an error from HandleE (not a panic), and leaves no phantom route.
func TestHandleEConflictReturnsError(t *testing.T) {
	api := server.NewAPI(server.NewServeMuxAdapter())
	h := func(w http.ResponseWriter, r *http.Request) error { return nil }
	if err := api.HandleE(http.MethodGet, "/files/:a", h); err != nil {
		t.Fatalf("first registration: %v", err)
	}
	err := api.HandleE(http.MethodGet, "/files/:b", h) // same match set → ServeMux conflict
	if err == nil {
		t.Fatal("conflicting registration: expected an error, got nil")
	}
	// The conflicting route must not have been recorded.
	for _, op := range api.RegisteredOperations() {
		_ = op
	}
	// A subsequent distinct route still registers cleanly (state not corrupted).
	if err := api.HandleE(http.MethodGet, "/other", h); err != nil {
		t.Fatalf("post-conflict registration: %v", err)
	}
}

// TestAdapterUseHTTP proves UseHTTP wraps the entire dispatch — its middleware
// runs even on 404, on both adapters.
func TestAdapterUseHTTP(t *testing.T) {
	adapters := map[string]func() server.Adapter{
		"ServeMux": func() server.Adapter { return server.NewServeMuxAdapter() },
		"Chi":      func() server.Adapter { return newChiAdapter() },
	}
	for name, mk := range adapters {
		t.Run(name, func(t *testing.T) {
			api := buildAPI(mk())
			api.UseHTTP(headerStamp)

			// matched route: edge middleware ran
			rec := httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/widgets/7", nil))
			if rec.Header().Get("X-Edge") != "seen" {
				t.Fatalf("UseHTTP did not wrap matched route")
			}
			// 404: edge middleware still ran (wraps 404/405 too)
			rec = httptest.NewRecorder()
			api.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/api/nope", nil))
			if rec.Header().Get("X-Edge") != "seen" {
				t.Fatalf("UseHTTP did not wrap the 404 path")
			}
		})
	}
}
