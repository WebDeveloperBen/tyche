package plugins_test

import (
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestCORS(t *testing.T) {
	t.Run("allows requests without Origin header", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.WriteHeader(http.StatusOK)
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("allows wildcard origin", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"*"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "*" {
			t.Errorf("expected *, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("allows specific origin", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
			t.Errorf("expected http://example.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("denies disallowed origin", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://evil.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "" {
			t.Errorf("expected empty, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("handles subdomain wildcard", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"*.example.com"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		for _, origin := range []string{"http://sub.example.com", "http://deep.sub.example.com", "http://sub-sub.example.com"} {
			req := httptest.NewRequest(http.MethodGet, "/test", nil)
			req.Header.Set("Origin", origin)
			w := httptest.NewRecorder()
			r.ServeHTTP(w, req)

			if w.Header().Get("Access-Control-Allow-Origin") != origin {
				t.Errorf("for origin %s: expected %s, got %s", origin, origin, w.Header().Get("Access-Control-Allow-Origin"))
			}
		}
	})

	t.Run("denies non-matching subdomain wildcard", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"*.example.com"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusOK {
			t.Errorf("expected 200, got %d", w.Code)
		}
	})

	t.Run("uses AllowOriginFunc callback", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedMethods: []string{http.MethodGet},
			AllowOriginFunc: func(r *http.Request, origin string) bool {
				return origin == "http://allowed.com"
			},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://allowed.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Allow-Origin") != "http://allowed.com" {
			t.Errorf("expected http://allowed.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
	})

	t.Run("preflight request returns proper headers", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins:   []string{"http://example.com"},
			AllowedMethods:   []string{http.MethodGet, http.MethodPost},
			AllowedHeaders:   []string{"Authorization", "Content-Type"},
			AllowCredentials: true,
			MaxAge:           86400,
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "POST")
		req.Header.Set("Access-Control-Request-Headers", "Authorization, Content-Type")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
		if w.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
			t.Errorf("expected http://example.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}
		if w.Header().Get("Access-Control-Allow-Methods") != "POST" {
			t.Errorf("expected POST, got %s", w.Header().Get("Access-Control-Allow-Methods"))
		}
		if w.Header().Get("Access-Control-Allow-Credentials") != "true" {
			t.Errorf("expected true, got %s", w.Header().Get("Access-Control-Allow-Credentials"))
		}
		if w.Header().Get("Access-Control-Max-Age") != "86400" {
			t.Errorf("expected 86400, got %s", w.Header().Get("Access-Control-Max-Age"))
		}
	})

	t.Run("preflight with disallowed method returns 204 without method header", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
			AllowedMethods: []string{http.MethodGet},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "DELETE")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}

		if w.Header().Get("Access-Control-Allow-Origin") != "http://example.com" {
			t.Errorf("expected http://example.com, got %s", w.Header().Get("Access-Control-Allow-Origin"))
		}

		if w.Header().Get("Access-Control-Allow-Methods") != "" {
			t.Errorf("expected no Access-Control-Allow-Methods for disallowed method, got %s", w.Header().Get("Access-Control-Allow-Methods"))
		}
	})

	t.Run("sets Vary header", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS()
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Vary") == "" {
			t.Error("expected Vary header to be set")
		}
	})

	t.Run("exposes headers with Access-Control-Expose-Headers", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedOrigins: []string{"http://example.com"},
			ExposedHeaders: []string{"X-Custom-Header"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Access-Control-Expose-Headers") != "X-Custom-Header" {
			t.Errorf("expected X-Custom-Header, got %s", w.Header().Get("Access-Control-Expose-Headers"))
		}
	})

	t.Run("canonicalizes header names", func(t *testing.T) {
		r := server.NewAPI(server.NewServeMuxAdapter())
		mw, err := plugins.CORS(plugins.CORSConfig{
			AllowedHeaders: []string{"x-custom-header", "content-type"},
		})
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		r.UseHTTP(mw)

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			return nil
		})

		req := httptest.NewRequest(http.MethodOptions, "/test", nil)
		req.Header.Set("Origin", "http://example.com")
		req.Header.Set("Access-Control-Request-Method", "GET")
		req.Header.Set("Access-Control-Request-Headers", "x-custom-header")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNoContent {
			t.Errorf("expected 204, got %d", w.Code)
		}
	})
}

func TestCORSWildcardWithCredentialsError(t *testing.T) {
	_, err := plugins.CORS(plugins.CORSConfig{
		AllowedOrigins:   []string{"*"},
		AllowCredentials: true,
	})

	if err == nil {
		t.Error("expected error for wildcard origin with credentials")
	}
	if !errors.Is(err, plugins.ErrCORSWildcardWithCredentials) {
		t.Errorf("expected ErrCORSWildcardWithCredentials, got %v", err)
	}
}
