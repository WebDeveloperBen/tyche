package server_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
)

func TestRouter_BasicRouting(t *testing.T) {
	router := server.NewRouter()

	var called bool
	router.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		called = true
		_, _ = w.Write([]byte("OK"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if w.Body.String() != "OK" {
		t.Errorf("expected body 'OK', got %s", w.Body.String())
	}
}

func TestRouter_NotFound(t *testing.T) {
	router := server.NewRouter()

	router.GET("/exists", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/notfound", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", w.Code)
	}
}

func TestRouter_AllMethods(t *testing.T) {
	router := server.NewRouter()

	methods := []string{
		http.MethodGet,
		http.MethodPost,
		http.MethodPut,
		http.MethodDelete,
		http.MethodPatch,
	}

	for _, method := range methods {
		t.Run(method, func(t *testing.T) {
			var called bool
			pattern := "/" + method

			switch method {
			case http.MethodGet:
				router.GET(pattern, func(w http.ResponseWriter, r *http.Request) error {
					called = true
					return nil
				})
			case http.MethodPost:
				router.POST(pattern, func(w http.ResponseWriter, r *http.Request) error {
					called = true
					return nil
				})
			case http.MethodPut:
				router.PUT(pattern, func(w http.ResponseWriter, r *http.Request) error {
					called = true
					return nil
				})
			case http.MethodDelete:
				router.DELETE(pattern, func(w http.ResponseWriter, r *http.Request) error {
					called = true
					return nil
				})
			case http.MethodPatch:
				router.PATCH(pattern, func(w http.ResponseWriter, r *http.Request) error {
					called = true
					return nil
				})
			}

			req := httptest.NewRequest(method, pattern, nil)
			w := httptest.NewRecorder()
			router.ServeHTTP(w, req)

			if !called {
				t.Errorf("handler for %s was not called", method)
			}
		})
	}
}

func TestRouter_MethodMismatch(t *testing.T) {
	router := server.NewRouter()

	router.POST("/resource", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/resource", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusMethodNotAllowed {
		t.Errorf("expected status 405 for method mismatch, got %d", w.Code)
	}
}

func TestRouter_RootPath(t *testing.T) {
	router := server.NewRouter()

	var called bool
	router.GET("/", func(w http.ResponseWriter, r *http.Request) error {
		called = true
		_, _ = w.Write([]byte("root"))
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !called {
		t.Error("root handler was not called")
	}
	if w.Body.String() != "root" {
		t.Errorf("expected body 'root', got %s", w.Body.String())
	}
}

func TestRouter_EmptyPattern(t *testing.T) {
	defer func() {
		if r := recover(); r == nil {
			t.Error("expected panic for path not starting with /")
		}
	}()

	router := server.NewRouter()
	router.GET("test", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})
}
