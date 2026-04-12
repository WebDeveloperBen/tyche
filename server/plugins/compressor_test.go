package plugins_test

import (
	"bytes"
	"compress/gzip"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/andybalholm/brotli"
	"github.com/webdeveloperben/tyche/server"
	"github.com/webdeveloperben/tyche/server/plugins"
)

func TestCompressor(t *testing.T) {
	t.Run("no compression when no Accept-Encoding", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Hello, World!"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected no Content-Encoding, got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("gzip compression", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Hello, World!"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("expected gzip, got %s", w.Header().Get("Content-Encoding"))
		}

		if w.Header().Get("Vary") != "Accept-Encoding" {
			t.Errorf("expected Accept-Encoding in Vary, got %s", w.Header().Get("Vary"))
		}

		gr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
		if err != nil {
			t.Fatalf("failed to create gzip reader: %v", err)
		}
		defer gr.Close()

		data, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("failed to read gzip data: %v", err)
		}

		if string(data) != "Hello, World!" {
			t.Errorf("expected 'Hello, World!', got '%s'", string(data))
		}
	})

	t.Run("brotli compression", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Hello, World!"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "br" {
			t.Errorf("expected br, got %s", w.Header().Get("Content-Encoding"))
		}

		br := brotli.NewReader(bytes.NewReader(w.Body.Bytes()))
		data, err := io.ReadAll(br)
		if err != nil {
			t.Fatalf("failed to read brotli data: %v", err)
		}

		if string(data) != "Hello, World!" {
			t.Errorf("expected 'Hello, World!', got '%s'", string(data))
		}
	})

	t.Run("brotli preferred over gzip", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("Hello, World!"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip, br")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "br" {
			t.Errorf("expected br (preferred), got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("no compression for non-compressible content type", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "application/octet-stream")
			w.Write([]byte("binary data"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected no Content-Encoding, got %s", w.Header().Get("Content-Encoding"))
		}

		if w.Body.String() != "binary data" {
			t.Errorf("expected uncompressed body")
		}
	})

	t.Run("wildcard content type matching", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			ContentTypes: []string{"text/*", "application/json"},
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/html")
			w.Write([]byte("<html>test</html>"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("expected gzip for text/html, got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("handles already set Content-Encoding", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			w.Header().Set("Content-Encoding", "identity")
			w.Write([]byte("Hello"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "identity" {
			t.Errorf("expected identity (already set), got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("Flush works with compression", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			w.Write([]byte("chunk1"))
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)
	})

	t.Run("empty response is not compressed", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") == "gzip" {
			t.Error("empty response should not be compressed")
		}
	})
}
