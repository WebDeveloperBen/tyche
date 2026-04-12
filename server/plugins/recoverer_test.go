package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestRecoverer(t *testing.T) {
	t.Run("recovers from panic", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Recoverer())

		r.GET("/panic", func(w http.ResponseWriter, r *http.Request) error {
			panic("test panic")
		})

		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Errorf("expected 500, got %d", w.Code)
		}
	})

	t.Run("normal request passes through", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Recoverer())

		var called bool
		r.GET("/ok", func(w http.ResponseWriter, r *http.Request) error {
			called = true
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/ok", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if !called {
			t.Error("handler should be called")
		}
		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("subsequent requests work after panic", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Recoverer())

		var subsequentCalled bool

		r.GET("/panic", func(w http.ResponseWriter, r *http.Request) error {
			panic("test panic")
		})
		r.GET("/subsequent", func(w http.ResponseWriter, r *http.Request) error {
			subsequentCalled = true
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/panic", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		req = httptest.NewRequest(http.MethodGet, "/subsequent", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("subsequent request: expected 200, got %d", w.Code)
		}
		if !subsequentCalled {
			t.Error("subsequent request should be called")
		}
	})
}
