package server_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func recordMW(order *[]string, label string) server.Middleware {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			*order = append(*order, label+"-before")
			err := next(w, r)
			*order = append(*order, label+"-after")
			return err
		}
	}
}

func TestMiddlewareFromFunc(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var order []string
	mw := server.MiddlewareFromFunc(func(w http.ResponseWriter, r *http.Request, next server.HandlerFunc) error {
		order = append(order, "mw-before")
		err := next(w, r)
		order = append(order, "mw-after")
		return err
	})

	g := router.Group("")
	g.Use(mw)
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	assertOrder(t, order, []string{"mw-before", "handler", "mw-after"})
}

func TestChain_Order(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var order []string
	chained := server.Chain(
		recordMW(&order, "a"),
		recordMW(&order, "b"),
		recordMW(&order, "c"),
	)

	g := router.Group("")
	g.Use(chained)
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	assertOrder(t, order, []string{
		"a-before", "b-before", "c-before",
		"handler",
		"c-after", "b-after", "a-after",
	})
}

type namedMW struct {
	name  string
	order *[]string
}

func (n namedMW) Name() string { return n.name }
func (n namedMW) Middleware() server.Middleware {
	return recordMW(n.order, n.name)
}

func TestUseNamed(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var order []string
	router.UseNamed(
		namedMW{name: "first", order: &order},
		namedMW{name: "second", order: &order},
	)

	router.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	router.ServeHTTP(httptest.NewRecorder(), req)

	assertOrder(t, order, []string{
		"first-before", "second-before",
		"handler",
		"second-after", "first-after",
	})
}

func TestContextKey_RoundTrip(t *testing.T) {
	type claims struct{ Subject string }
	key := server.NewContextKey[claims]("auth")

	router := server.NewAPI(server.NewServeMuxAdapter())
	router.Use(func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			return next(w, key.WithRequest(r, claims{Subject: "user-42"}))
		}
	})

	var got claims
	var ok bool
	router.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		got, ok = key.From(r.Context())
		return nil
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))

	if !ok {
		t.Fatal("expected value to be present in context")
	}
	if got.Subject != "user-42" {
		t.Errorf("expected subject 'user-42', got %q", got.Subject)
	}
}

func TestContextKey_MissingAndDistinct(t *testing.T) {
	a := server.NewContextKey[string]("a")
	b := server.NewContextKey[string]("b")

	ctx := a.WithValue(context.Background(), "value-a")

	if _, ok := b.From(ctx); ok {
		t.Error("distinct key should not read another key's value")
	}
	if v, ok := a.From(ctx); !ok || v != "value-a" {
		t.Errorf("expected ('value-a', true), got (%q, %v)", v, ok)
	}
}

func TestWithMiddleware_RunsAfterGroup(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var order []string
	g := router.Group("", recordMW(&order, "group"))
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	}, server.WithMiddleware(recordMW(&order, "route")))

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))

	assertOrder(t, order, []string{
		"group-before", "route-before",
		"handler",
		"route-after", "group-after",
	})
}

func TestWithMiddleware_IsolatedPerRoute(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var order []string
	g := router.Group("", recordMW(&order, "group"))
	g.GET("/guarded", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "guarded")
		return nil
	}, server.WithMiddleware(recordMW(&order, "route")))
	g.GET("/open", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "open")
		return nil
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/open", nil))

	assertOrder(t, order, []string{"group-before", "open", "group-after"})
}

func TestWithMiddleware_SurvivesLateGroupUse(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var order []string
	g := router.Group("")
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	}, server.WithMiddleware(recordMW(&order, "route")))

	// Registering group middleware after the route triggers a rebuild; the
	// route-level middleware must still be applied.
	g.Use(recordMW(&order, "group"))

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/test", nil))

	assertOrder(t, order, []string{
		"group-before", "route-before",
		"handler",
		"route-after", "group-after",
	})
}

func assertOrder(t *testing.T, got, want []string) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("expected order %v, got %v", want, got)
	}
	for i := range want {
		if got[i] != want[i] {
			t.Errorf("at position %d: expected %q, got %q", i, want[i], got[i])
		}
	}
}
