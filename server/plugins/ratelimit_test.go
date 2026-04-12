package plugins_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestRateLimit(t *testing.T) {
	t.Run("allows requests within limit", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 10,
			Burst:             10,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 10 {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("returns 429 when limit exceeded", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             1,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req1 := httptest.NewRequest(http.MethodGet, "/test", nil)
		w1 := httptest.NewRecorder()
		r.ServeHTTP(w1, req1)

		if w1.Code != http.StatusOK {
			t.Errorf("first request: expected 200, got %d", w1.Code)
		}

		req2 := httptest.NewRequest(http.MethodGet, "/test", nil)
		w2 := httptest.NewRecorder()
		r.ServeHTTP(w2, req2)

		if w2.Code != http.StatusTooManyRequests {
			t.Errorf("second request: expected 429, got %d", w2.Code)
		}
	})

	t.Run("token bucket refills over time", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 100,
			Burst:             1,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("first request: expected 200, got %d", w.Code)
		}

		req = httptest.NewRequest(http.MethodGet, "/test", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("second request (immediate): expected 429, got %d", w.Code)
		}

		time.Sleep(20 * time.Millisecond)

		req = httptest.NewRequest(http.MethodGet, "/test", nil)
		w = httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("third request (after refill): expected 200, got %d", w.Code)
		}
	})

	t.Run("uses default values", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RateLimit())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 200 {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("high burst allows burst of requests", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             100,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		for i := range 100 {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Code != http.StatusOK {
				t.Errorf("request %d: expected 200, got %d", i, w.Code)
			}
		}
	})

	t.Run("error message is 'Too Many Requests'", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RateLimit(plugins.RateLimitConfig{
			RequestsPerSecond: 1,
			Burst:             0,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusTooManyRequests {
			t.Errorf("expected 429, got %d", w.Code)
		}

		var payload struct {
			Error string `json:"error"`
		}
		if err := json.Unmarshal(w.Body.Bytes(), &payload); err != nil {
			t.Fatalf("expected JSON error response, got %v", err)
		}
		if payload.Error != "Too Many Requests" {
			t.Errorf("expected 'Too Many Requests', got '%s'", payload.Error)
		}
	})
}
