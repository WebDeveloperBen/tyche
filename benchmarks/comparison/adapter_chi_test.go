// Package comparison_test holds the cross-router benchmarks and reference
// adapter implementations that compare tyche's stdlib-backed router to chi,
// gin, and huma+chi. It is a separate Go module so those third-party router
// dependencies do not leak into the main tyche module's dep graph; running
// these tests is opt-in (cd into this directory and run `go test ./...`).
package comparison_test

import (
	"bufio"
	"context"
	"encoding/json"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/go-chi/chi/v5"
	"github.com/webdeveloperben/tyche/server"
)

// chiAdapter is a reference server.Adapter over github.com/go-chi/chi. It is
// kept here (not in the tyche core) so importing the server package never
// pulls chi into the build graph. Copy this file to use chi as a tyche
// router: it is the entire integration in ~40 lines, and the test below
// proves it works unchanged against the full tyche surface.
type chiAdapter struct{ mux chi.Router }

func newChiAdapter() *chiAdapter { return &chiAdapter{mux: chi.NewRouter()} }

var _ server.Adapter = (*chiAdapter)(nil)

func (a *chiAdapter) Handle(method, path string, h http.Handler) {
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

func (a *chiAdapter) SetFallback(notFound, methodNotAllowed http.Handler) {
	if notFound != nil {
		a.mux.NotFound(notFound.ServeHTTP)
	}
	if methodNotAllowed != nil {
		a.mux.MethodNotAllowed(methodNotAllowed.ServeHTTP)
	}
}

func (a *chiAdapter) ServeHTTP(w http.ResponseWriter, r *http.Request) { a.mux.ServeHTTP(w, r) }

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

type widgetInput struct {
	ID string `path:"id"`
}

type widgetOutput struct {
	Name string `json:"name"`
}

type streamInput struct {
	Topic string `query:"topic" required:"true"`
}

type streamToken struct {
	Text string `json:"text"`
}

var _ = (*bufio.ReadWriter)(nil) // keep bufio in case future helpers need it

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
	}, func(_ context.Context, in *streamInput, stream *server.Stream[streamToken]) error {
		for _, tok := range []string{in.Topic, "b", "c"} {
			if err := stream.Send(streamToken{Text: tok}); err != nil {
				return err
			}
		}
		return nil
	})

	_ = api.MountOpenAPI("/openapi.json")
	return api
}

// TestChiAdapter runs the same end-to-end checks the main module runs on
// ServeMux, but with chi as the routing backend. It exists to confirm the
// adapter contract is router-agnostic: a third-party router behaves the same
// as the stdlib one for typed routes, SSE streams, OpenAPI generation, 404,
// and 405.
func TestChiAdapter(t *testing.T) {
	api := buildAPI(newChiAdapter())

	// 1. Typed route: param binding + {"data":…} envelope.
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
}

var _ = net.Conn(nil) // keep the net import in case future tests need it
