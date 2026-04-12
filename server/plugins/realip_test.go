package plugins_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestRealIP(t *testing.T) {
	t.Run("uses direct client IP when no proxy", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP())

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "" {
			t.Errorf("expected empty X-Real-IP (direct client), got %s", realIP)
		}
	})

	t.Run("extracts IP from X-Forwarded-For", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP(plugins.RealIPConfig{
			TrustedProxies: []string{"127.0.0.1", "10.0.0.0/8"},
		}))

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.50, 10.0.0.1, 127.0.0.1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "203.0.113.50" {
			t.Errorf("expected 203.0.113.50, got %s", realIP)
		}
	})

	t.Run("strips trusted proxies from X-Forwarded-For", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP(plugins.RealIPConfig{
			TrustedProxies: []string{"10.0.0.0/8", "192.168.0.0/16"},
		}))

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.50, 192.168.1.1, 10.0.0.5")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "203.0.113.50" {
			t.Errorf("expected 203.0.113.50, got %s", realIP)
		}
	})

	t.Run("handles single IP in X-Forwarded-For", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP(plugins.RealIPConfig{
			TrustedProxies: []string{"127.0.0.1"},
		}))

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "198.51.100.178")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "198.51.100.178" {
			t.Errorf("expected 198.51.100.178, got %s", realIP)
		}
	})

	t.Run("empty X-Forwarded-For does not set X-Real-IP", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP())

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "" {
			t.Errorf("expected empty, got %s", realIP)
		}
	})

	t.Run("uses default trusted proxies", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP())

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", "203.0.113.50, 127.0.0.1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "203.0.113.50" {
			t.Errorf("expected 203.0.113.50, got %s", realIP)
		}
	})

	t.Run("handles whitespace in X-Forwarded-For", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.RealIP(plugins.RealIPConfig{
			TrustedProxies: []string{"10.0.0.0/8"},
		}))

		var realIP string
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			realIP = r.Header.Get("X-Real-IP")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("X-Forwarded-For", " 203.0.113.50 , 10.0.0.1 ")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if realIP != "203.0.113.50" {
			t.Errorf("expected 203.0.113.50, got '%s'", realIP)
		}
	})
}
