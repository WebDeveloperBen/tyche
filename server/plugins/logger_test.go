package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestLogger(t *testing.T) {
	t.Run("logs request without body or query by default", func(t *testing.T) {
		var logCalls []struct {
			method, path, query string
			status              int
			duration            time.Duration
			err                 error
		}

		r := server.NewRouter()
		r.Use(plugins.Logger(plugins.LoggerConfig{
			LogFunc: func(method, path, query string, status int, duration time.Duration, err error) {
				logCalls = append(logCalls, struct {
					method, path, query string
					status              int
					duration            time.Duration
					err                 error
				}{method, path, query, status, duration, err})
			},
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if len(logCalls) != 1 {
			t.Fatalf("expected 1 log call, got %d", len(logCalls))
		}

		if logCalls[0].method != "GET" {
			t.Errorf("expected method GET, got %s", logCalls[0].method)
		}
		if logCalls[0].path != "/test" {
			t.Errorf("expected path /test, got %s", logCalls[0].path)
		}
	})

	t.Run("logs query string when configured", func(t *testing.T) {
		var logCalls []struct {
			method, path, query string
			status              int
			duration            time.Duration
			err                 error
		}

		r := server.NewRouter()
		r.Use(plugins.Logger(plugins.LoggerConfig{
			WithQuery: true,
			LogFunc: func(method, path, query string, status int, duration time.Duration, err error) {
				logCalls = append(logCalls, struct {
					method, path, query string
					status              int
					duration            time.Duration
					err                 error
				}{method, path, query, status, duration, err})
			},
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test?foo=bar&baz=qux", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if len(logCalls) != 1 {
			t.Fatalf("expected 1 log call, got %d", len(logCalls))
		}

		if logCalls[0].query != "foo=bar&baz=qux" {
			t.Errorf("expected 'foo=bar&baz=qux', got '%s'", logCalls[0].query)
		}
	})

	t.Run("logs request body when configured", func(t *testing.T) {
		r := server.NewRouter()
		var loggedBody string

		r.Use(plugins.Logger(plugins.LoggerConfig{
			WithBody: true,
			LogFunc: func(method, path, query string, status int, duration time.Duration, err error) {
			},
		}))

		r.POST("/test", func(w http.ResponseWriter, r *http.Request) error {
			body := make([]byte, 1024)
			n, _ := r.Body.Read(body)
			w.WriteHeader(http.StatusOK)
			w.Write(body[:n])
			loggedBody = string(body[:n])
			return nil
		})

		req := httptest.NewRequest(http.MethodPost, "/test", strings.NewReader("test body"))
		req.Header.Set("Content-Type", "text/plain")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if loggedBody != "test body" {
			t.Errorf("expected 'test body', got '%s'", loggedBody)
		}
	})

	t.Run("measures duration", func(t *testing.T) {
		var loggedDuration time.Duration

		r := server.NewRouter()
		r.Use(plugins.Logger(plugins.LoggerConfig{
			DurationMs: true,
			LogFunc: func(method, path, query string, status int, duration time.Duration, err error) {
				loggedDuration = duration
			},
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			time.Sleep(10 * time.Millisecond)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if loggedDuration < 5*time.Millisecond {
			t.Errorf("expected duration >= 5ms, got %v", loggedDuration)
		}
	})

	t.Run("passes error to log function", func(t *testing.T) {
		var loggedErr error

		r := server.NewRouter()
		r.Use(plugins.Logger(plugins.LoggerConfig{
			LogFunc: func(method, path, query string, status int, duration time.Duration, err error) {
				loggedErr = err
			},
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return server.NewHTTPError(http.StatusNotFound, "not found")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if loggedErr == nil {
			t.Error("expected error to be logged")
		}
	})

	t.Run("writes response with correct status", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Logger(plugins.LoggerConfig{
			LogFunc: func(method, path, query string, status int, duration time.Duration, err error) {
			},
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusCreated)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusCreated {
			t.Errorf("expected 201, got %d", w.Code)
		}
	})

	t.Run("default LogFunc uses slog", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Logger())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	})
}
