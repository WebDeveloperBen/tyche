package server_test

import (
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

// TestUse_ConcurrentWithServing exercises the data race that previously existed
// when Use()/rebuildHandlers mutated a route's wrapped handler while in-flight
// requests were reading it. Run with -race; it must stay clean.
func TestUse_ConcurrentWithServing(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	g := router.Group("")
	g.GET("/x", func(w http.ResponseWriter, r *http.Request) error {
		w.WriteHeader(http.StatusOK)
		return nil
	})

	var wg sync.WaitGroup
	stop := make(chan struct{})

	// Concurrent request servers (readers of wrappedFn).
	for range 6 {
		wg.Go(func() {
			for {
				select {
				case <-stop:
					return
				default:
				}
				rec := httptest.NewRecorder()
				router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
				if rec.Code != http.StatusOK {
					t.Errorf("unexpected status %d", rec.Code)
					return
				}
			}
		})
	}

	// A single configuration goroutine adding middleware (writer of wrappedFn),
	// matching the realistic "reconfigure while serving" scenario.
	for range 100 {
		g.Use(func(next server.HandlerFunc) server.HandlerFunc { return next })
	}
	close(stop)
	wg.Wait()
}

func TestErrorHandler_Custom(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var gotPath string
	router.SetErrorHandler(func(w http.ResponseWriter, r *http.Request, err error) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusTeapot)
		_, _ = w.Write([]byte(`{"custom":"` + err.Error() + `"}`))
	})

	router.GET("/boom", func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("kaboom")
	})

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/boom", nil))

	if rec.Code != http.StatusTeapot {
		t.Fatalf("expected 418, got %d", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "kaboom") {
		t.Errorf("expected custom body, got %q", rec.Body.String())
	}
	if gotPath != "/boom" {
		t.Errorf("error handler did not receive request, path = %q", gotPath)
	}
}

func TestErrorHandler_ConfiguredViaConfig(t *testing.T) {
	called := false
	router := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{
		ErrorHandler: func(w http.ResponseWriter, r *http.Request, err error) {
			called = true
			w.WriteHeader(http.StatusBadGateway)
		},
	})
	router.GET("/x", func(w http.ResponseWriter, r *http.Request) error {
		return errors.New("nope")
	})
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/x", nil))
	if !called || rec.Code != http.StatusBadGateway {
		t.Fatalf("config error handler not used: called=%v code=%d", called, rec.Code)
	}
}

func TestNotFound_DefaultProblemJSON(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))

	if rec.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("expected problem+json, got %q", ct)
	}
}

func TestNotFound_Custom(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	router.SetNotFoundHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusGone)
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/missing", nil))
	if rec.Code != http.StatusGone {
		t.Fatalf("expected 410, got %d", rec.Code)
	}
}

func TestMethodNotAllowed_DefaultProblemJSON(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	router.GET("/only-get", func(w http.ResponseWriter, r *http.Request) error { return nil })

	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodPost, "/only-get", nil))

	if rec.Code != http.StatusMethodNotAllowed {
		t.Fatalf("expected 405, got %d", rec.Code)
	}
	if ct := rec.Header().Get("Content-Type"); ct != "application/problem+json" {
		t.Errorf("expected problem+json, got %q", ct)
	}
}

func TestMethodNotAllowed_Custom(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	router.GET("/only-get", func(w http.ResponseWriter, r *http.Request) error { return nil })
	router.SetMethodNotAllowedHandler(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Allow", "GET")
		w.WriteHeader(http.StatusMethodNotAllowed)
		_, _ = w.Write([]byte("nope"))
	}))
	rec := httptest.NewRecorder()
	router.ServeHTTP(rec, httptest.NewRequest(http.MethodDelete, "/only-get", nil))
	if rec.Body.String() != "nope" || rec.Header().Get("Allow") != "GET" {
		t.Errorf("custom 405 handler not used: body=%q allow=%q", rec.Body.String(), rec.Header().Get("Allow"))
	}
}

func readBodyHandler(w http.ResponseWriter, r *http.Request) error {
	if _, err := io.ReadAll(r.Body); err != nil {
		return server.NewHTTPError(http.StatusRequestEntityTooLarge, "too large")
	}
	w.WriteHeader(http.StatusOK)
	return nil
}

func TestWithMaxBodyBytes_OverridesGlobal(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter(), server.APIConfig{MaxRequestBodyBytes: 8})

	// Default route inherits the tiny global limit.
	router.POST("/small", readBodyHandler)
	// This route opts out of the limit entirely.
	router.POST("/large", readBodyHandler, server.WithMaxBodyBytes(0))
	// This route raises the limit.
	router.POST("/medium", readBodyHandler, server.WithMaxBodyBytes(1024))

	big := strings.NewReader(strings.Repeat("x", 64))

	recSmall := httptest.NewRecorder()
	router.ServeHTTP(recSmall, httptest.NewRequest(http.MethodPost, "/small", strings.NewReader(strings.Repeat("x", 64))))
	if recSmall.Code != http.StatusRequestEntityTooLarge {
		t.Errorf("/small: expected 413, got %d", recSmall.Code)
	}

	recLarge := httptest.NewRecorder()
	router.ServeHTTP(recLarge, httptest.NewRequest(http.MethodPost, "/large", big))
	if recLarge.Code != http.StatusOK {
		t.Errorf("/large (unlimited): expected 200, got %d", recLarge.Code)
	}

	recMedium := httptest.NewRecorder()
	router.ServeHTTP(recMedium, httptest.NewRequest(http.MethodPost, "/medium", strings.NewReader(strings.Repeat("x", 64))))
	if recMedium.Code != http.StatusOK {
		t.Errorf("/medium (1024): expected 200, got %d", recMedium.Code)
	}
}

func TestMount_ServesPrefixAndSubpaths(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	sub := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("mounted:" + r.URL.Path)) //nolint:gosec
	})
	if err := router.Mount("/debug", sub); err != nil {
		t.Fatalf("Mount: %v", err)
	}

	for _, path := range []string{"/debug", "/debug/pprof", "/debug/pprof/heap"} {
		rec := httptest.NewRecorder()
		router.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, path, nil))
		if rec.Code != http.StatusOK {
			t.Errorf("%s: expected 200, got %d", path, rec.Code)
		}
		if want := "mounted:" + path; rec.Body.String() != want {
			t.Errorf("%s: expected body %q, got %q", path, want, rec.Body.String())
		}
	}
}

func TestMount_RejectsBadPrefix(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())
	if err := router.Mount("debug", http.NotFoundHandler()); err == nil {
		t.Error("expected error for prefix without leading slash")
	}
	if err := router.Mount("/", http.NotFoundHandler()); err == nil {
		t.Error("expected error for root prefix")
	}
	if err := router.Mount("/ok", nil); err == nil {
		t.Error("expected error for nil handler")
	}
}

func TestRoutePattern(t *testing.T) {
	router := server.NewAPI(server.NewServeMuxAdapter())

	var pattern string
	router.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		pattern = server.RoutePattern(r)
		return nil
	})

	router.ServeHTTP(httptest.NewRecorder(), httptest.NewRequest(http.MethodGet, "/users/42", nil))
	if pattern != "/users/:id" {
		t.Errorf("expected route pattern '/users/:id', got %q", pattern)
	}
}
