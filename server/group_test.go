package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestGroup_Basic(t *testing.T) {
	router := NewRouter()

	var called bool
	g := router.Group("/api")
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		called = true
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called")
	}
	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
}

func TestGroup_Nested(t *testing.T) {
	router := NewRouter()

	var pathInHandler string
	g1 := router.Group("/api")
	g2 := g1.Group("/v1")
	g3 := g2.Group("/users")

	g3.GET("/:id", func(w http.ResponseWriter, r *http.Request) error {
		pathInHandler = Param(r, "id")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", w.Code)
	}
	if pathInHandler != "123" {
		t.Errorf("expected id '123', got %s", pathInHandler)
	}
}

func TestGroup_Middleware(t *testing.T) {
	router := NewRouter()

	var order []string

	middleware := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			order = append(order, "middleware")
			return next(w, r)
		}
	}

	g := router.Group("/api", middleware)
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expected := []string{"middleware", "handler"}
	if len(order) != len(expected) {
		t.Errorf("expected order %v, got %v", expected, order)
	}
}

func TestGroup_MultipleGroups(t *testing.T) {
	router := NewRouter()

	var api1Called, api2Called bool

	g1 := router.Group("/api/v1")
	g1.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		api1Called = true
		return nil
	})

	g2 := router.Group("/api/v2")
	g2.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		api2Called = true
		return nil
	})

	req1 := httptest.NewRequest(http.MethodGet, "/api/v1/test", nil)
	w1 := httptest.NewRecorder()
	router.ServeHTTP(w1, req1)

	if !api1Called {
		t.Error("api/v1 handler was not called")
	}

	req2 := httptest.NewRequest(http.MethodGet, "/api/v2/test", nil)
	w2 := httptest.NewRecorder()
	router.ServeHTTP(w2, req2)

	if !api2Called {
		t.Error("api/v2 handler was not called")
	}
}

func TestGroup_EmptyPrefix(t *testing.T) {
	router := NewRouter()

	var called bool
	g := router.Group("")
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		called = true
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if !called {
		t.Error("handler was not called")
	}
}

func TestGroup_AllHTTPMethods(t *testing.T) {
	router := NewRouter()

	g := router.Group("/api")

	methods := []string{http.MethodGet, http.MethodPost, http.MethodPut, http.MethodDelete, http.MethodPatch}
	calls := make(map[string]bool)

	for _, method := range methods {
		path := "/api/" + method
		switch method {
		case http.MethodGet:
			g.GET("/"+method, func(w http.ResponseWriter, r *http.Request) error {
				calls[method] = true
				return nil
			})
		case http.MethodPost:
			g.POST("/"+method, func(w http.ResponseWriter, r *http.Request) error {
				calls[method] = true
				return nil
			})
		case http.MethodPut:
			g.PUT("/"+method, func(w http.ResponseWriter, r *http.Request) error {
				calls[method] = true
				return nil
			})
		case http.MethodDelete:
			g.DELETE("/"+method, func(w http.ResponseWriter, r *http.Request) error {
				calls[method] = true
				return nil
			})
		case http.MethodPatch:
			g.PATCH("/"+method, func(w http.ResponseWriter, r *http.Request) error {
				calls[method] = true
				return nil
			})
		}

		req := httptest.NewRequest(method, path, nil)
		w := httptest.NewRecorder()
		router.ServeHTTP(w, req)

		if !calls[method] {
			t.Errorf("method %s handler was not called", method)
		}
	}
}

func TestGroup_UseAddsMiddleware(t *testing.T) {
	router := NewRouter()

	var order []string

	mw1 := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			order = append(order, "mw1")
			return next(w, r)
		}
	}
	mw2 := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			order = append(order, "mw2")
			return next(w, r)
		}
	}

	g := router.Group("/api", mw1)
	g.Use(mw2)
	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expected := []string{"mw2", "mw1", "handler"}
	if len(order) != len(expected) {
		t.Errorf("expected order %v, got %v", expected, order)
	}
}

func TestGroup_SiblingMiddlewareDoesNotLeak(t *testing.T) {
	router := NewRouter()
	var calls []string

	api := router.Group("/api", func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			calls = append(calls, "api")
			return next(w, r)
		}
	})
	admin := router.Group("/admin", func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			calls = append(calls, "admin")
			return next(w, r)
		}
	})

	api.GET("/users", func(w http.ResponseWriter, r *http.Request) error {
		calls = append(calls, "users")
		return nil
	})
	admin.GET("/stats", func(w http.ResponseWriter, r *http.Request) error {
		calls = append(calls, "stats")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/api/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expected := []string{"api", "users"}
	if len(calls) != len(expected) {
		t.Fatalf("expected calls %v, got %v", expected, calls)
	}
	for i := range expected {
		if calls[i] != expected[i] {
			t.Fatalf("expected calls %v, got %v", expected, calls)
		}
	}
}

func TestGroup_ParentUseAfterChildCreationRebuildsHandlers(t *testing.T) {
	router := NewRouter()
	var calls []string

	parent := router.Group("/api")
	child := parent.Group("/v1")
	child.GET("/users", func(w http.ResponseWriter, r *http.Request) error {
		calls = append(calls, "handler")
		return nil
	})

	parent.Use(func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			calls = append(calls, "parent")
			return next(w, r)
		}
	})

	req := httptest.NewRequest(http.MethodGet, "/api/v1/users", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expected := []string{"parent", "handler"}
	if len(calls) != len(expected) {
		t.Fatalf("expected calls %v, got %v", expected, calls)
	}
	for i := range expected {
		if calls[i] != expected[i] {
			t.Fatalf("expected calls %v, got %v", expected, calls)
		}
	}
}
