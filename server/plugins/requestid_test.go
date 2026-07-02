package plugins_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/google/uuid"
	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestRequestID(t *testing.T) {
	t.Run("generates UUID for request", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.Use(plugins.RequestID())

		var requestID string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			requestID = plugins.RequestIDFromContext(r.Context())
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if requestID == "" {
			t.Error("request ID should not be empty")
		}

		if _, err := uuid.Parse(requestID); err != nil {
			t.Errorf("request ID should be valid UUID: %v", err)
		}
	})

	t.Run("returns X-Request-ID header", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.Use(plugins.RequestID())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("X-Request-ID") == "" {
			t.Error("X-Request-ID header should be set")
		}

		if _, err := uuid.Parse(w.Header().Get("X-Request-ID")); err != nil {
			t.Errorf("X-Request-ID should be valid UUID: %v", err)
		}
	})

	t.Run("uses client-provided request ID", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.Use(plugins.RequestID())

		clientID := "client-request-123"

		var ctxID string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			ctxID = plugins.RequestIDFromContext(r.Context())
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-ID", clientID)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if ctxID != clientID {
			t.Errorf("expected %s, got %s", clientID, ctxID)
		}

		if w.Header().Get("X-Request-ID") != clientID {
			t.Errorf("expected %s, got %s", clientID, w.Header().Get("X-Request-ID"))
		}
	})

	t.Run("uses custom header name", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.Use(plugins.RequestID(plugins.RequestIDConfig{
			HeaderName: "X-Request-Context",
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Request-Context", "custom-id")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("X-Request-Context") != "custom-id" {
			t.Errorf("expected custom-id, got %s", w.Header().Get("X-Request-Context"))
		}

		if w.Header().Get("X-Request-ID") != "" {
			t.Error("default header X-Request-ID should not be set")
		}
	})

	t.Run("RequestIDFromContext returns empty for missing", func(t *testing.T) {
		id := plugins.RequestIDFromContext(context.Background())
		if id != "" {
			t.Errorf("expected empty string, got %s", id)
		}
	})

	t.Run("header is not duplicated if already set", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		r.Use(plugins.RequestID())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("X-Request-ID", "already-set")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("X-Request-ID") != "already-set" {
			t.Errorf("expected 'already-set', got %s", w.Header().Get("X-Request-ID"))
		}
	})
}
