package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestMiddleware_Basic(t *testing.T) {
	router := NewRouter()

	var order []string

	middleware := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			order = append(order, "before")
			err := next(w, r)
			order = append(order, "after")
			return err
		}
	}

	g := router.Group("")
	g.Use(middleware)

	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expected := []string{"before", "handler", "after"}
	if len(order) != len(expected) {
		t.Errorf("expected order %v, got %v", expected, order)
	}
	for i, e := range expected {
		if order[i] != e {
			t.Errorf("at position %d: expected %s, got %s", i, e, order[i])
		}
	}
}

func TestMiddleware_Multiple(t *testing.T) {
	router := NewRouter()

	var order []string

	mw1 := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			order = append(order, "mw1-before")
			return next(w, r)
		}
	}

	mw2 := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			order = append(order, "mw2-before")
			return next(w, r)
		}
	}

	g := router.Group("", mw1, mw2)

	g.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
		order = append(order, "handler")
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/test", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	expected := []string{"mw1-before", "mw2-before", "handler"}
	if len(order) != len(expected) {
		t.Errorf("expected order %v, got %v", expected, order)
	}
	for i, e := range expected {
		if order[i] != e {
			t.Errorf("at position %d: expected %s, got %s", i, e, order[i])
		}
	}
}

func TestMiddleware_ErrorInHandler(t *testing.T) {
	router := NewRouter()

	middleware := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			err := next(w, r)
			if err != nil {
				if httpErr, ok := err.(HTTPError); ok {
					http.Error(w, httpErr.Message, httpErr.StatusCode)
				}
			}
			return err
		}
	}

	g := router.Group("")
	g.Use(middleware)

	g.GET("/error", func(w http.ResponseWriter, r *http.Request) error {
		return NewHTTPError(400, "Invalid input")
	})

	req := httptest.NewRequest(http.MethodGet, "/error", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected status 400, got %d", w.Code)
	}
}

func TestMiddleware_RequestContext(t *testing.T) {
	router := NewRouter()

	var idInHandler string

	middleware := func(next HandlerFunc) HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			idInHandler = Param(r, "id")
			return next(w, r)
		}
	}

	g := router.Group("")
	g.Use(middleware)

	g.GET("/users/:id", func(w http.ResponseWriter, r *http.Request) error {
		return nil
	})

	req := httptest.NewRequest(http.MethodGet, "/users/123", nil)
	w := httptest.NewRecorder()
	router.ServeHTTP(w, req)

	if idInHandler != "123" {
		t.Errorf("expected id '123', got %q", idInHandler)
	}
}
