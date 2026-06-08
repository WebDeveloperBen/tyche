package plugins_test

import (
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
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
			_, _ = w.Write([]byte("Hello, World!"))
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
			_, _ = w.Write([]byte("Hello, World!"))
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
		defer func() { _ = gr.Close() }()

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
			_, _ = w.Write([]byte("Hello, World!"))
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
			_, _ = w.Write([]byte("Hello, World!"))
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

	t.Run("quality values are respected", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Hello, World!"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "br;q=0.4, gzip;q=0.9")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "gzip" {
			t.Errorf("expected gzip, got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("wildcard can beat explicit lower quality encoding", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Hello, World!"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip;q=0.1, *;q=1")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "br" {
			t.Errorf("expected br from wildcard preference, got %s", w.Header().Get("Content-Encoding"))
		}
	})

	t.Run("no compression for non-compressible content type", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "application/octet-stream")
			_, _ = w.Write([]byte("binary data"))
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
			_, _ = w.Write([]byte("<html>test</html>"))
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
			_, _ = w.Write([]byte("Hello"))
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
			_, _ = w.Write([]byte("chunk1"))
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

	t.Run("gzip compression respects MaxCompressedSize", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize: 40,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Hello, World! This is more than 10 bytes."))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected fallback to identity, got %s", w.Header().Get("Content-Encoding"))
		}

		if w.Body.String() != "Hello, World! This is more than 10 bytes." {
			t.Errorf("expected original body on fallback, got %q", w.Body.String())
		}
	})

	t.Run("brotli compression respects MaxCompressedSize", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize: 20,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Hello, World! This is more than 10 bytes."))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "br")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Header().Get("Content-Encoding") != "" {
			t.Errorf("expected fallback to identity, got %s", w.Header().Get("Content-Encoding"))
		}

		if w.Body.String() != "Hello, World! This is more than 10 bytes." {
			t.Errorf("expected original body on fallback, got %q", w.Body.String())
		}
	})

	t.Run("small responses under MaxCompressedSize are not truncated", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize: 100,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Hi"))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		gr, err := gzip.NewReader(bytes.NewReader(w.Body.Bytes()))
		if err != nil {
			t.Fatalf("failed to create gzip reader: %v", err)
		}
		defer func() { _ = gr.Close() }()

		data, err := io.ReadAll(gr)
		if err != nil {
			t.Fatalf("failed to read gzip data: %v", err)
		}

		if string(data) != "Hi" {
			t.Errorf("expected 'Hi', got '%s'", string(data))
		}
	})

	t.Run("buffered mode discards partial body when handler returns error", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor())

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("partial response"))
			return errors.New("boom")
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusInternalServerError {
			t.Fatalf("expected 500, got %d", w.Code)
		}
		if strings.Contains(w.Body.String(), "partial response") {
			t.Fatalf("expected buffered partial body to be discarded, got %q", w.Body.String())
		}
		if got := w.Header().Get("Content-Encoding"); got != "" {
			t.Fatalf("expected no compression header on error response, got %s", got)
		}
	})

	t.Run("large fallback preserves committed headers", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize: 64,
		}))

		body := pseudoRandomASCII(2 << 20)
		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte(body))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := newStrictHeaderRecorder()
		r.ServeHTTP(w, req)

		if got := w.Result().Header.Get("Content-Encoding"); got != "" {
			t.Fatalf("expected identity fallback on live server, got %s", got)
		}
		if got := w.Result().Header.Get("Content-Length"); got != strconv.Itoa(len(body)) {
			t.Fatalf("expected Content-Length %d, got %s", len(body), got)
		}
		if w.Body.String() != body {
			t.Fatalf("expected original body after fallback")
		}
	})

	t.Run("buffer overflow switches to identity passthrough", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize:       128,
			MaxBufferedResponseSize: 32,
		}))

		partA := "0123456789abcdef"
		partB := "ghijklmnopqrstuvwxyz012345"
		expected := partA + partB

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			if _, err := w.Write([]byte(partA)); err != nil {
				return err
			}
			if _, err := w.Write([]byte(partB)); err != nil {
				return err
			}
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := newStrictHeaderRecorder()
		r.ServeHTTP(w, req)

		if got := w.Result().Header.Get("Content-Encoding"); got != "" {
			t.Fatalf("expected identity passthrough, got %s", got)
		}
		if got := w.Result().Header.Get("Content-Length"); got != "" {
			t.Fatalf("expected no Content-Length after passthrough switch, got %s", got)
		}
		if w.Body.String() != expected {
			t.Fatalf("expected full identity body after passthrough, got %q", w.Body.String())
		}
	})

	t.Run("identity forbidden returns not acceptable on size fallback", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize: 40,
		}))

		r.GET("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, _ = w.Write([]byte("Hello, World! This is more than 10 bytes."))
			return nil
		})

		req := httptest.NewRequest(http.MethodGet, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip, identity;q=0")
		w := httptest.NewRecorder()
		r.ServeHTTP(w, req)

		if w.Code != http.StatusNotAcceptable {
			t.Fatalf("expected 406, got %d", w.Code)
		}
	})

	t.Run("head overflow does not emit a body", func(t *testing.T) {
		r := server.NewRouter()
		r.Use(plugins.Compressor(plugins.CompressorConfig{
			MaxCompressedSize:       128,
			MaxBufferedResponseSize: 32,
		}))

		r.HEAD("/test", func(w http.ResponseWriter, r *http.Request) error {
			w.Header().Set("Content-Type", "text/plain")
			_, err := w.Write([]byte("0123456789abcdefghijklmnopqrstuvwxyz"))
			return err
		})

		req := httptest.NewRequest(http.MethodHead, "/test", nil)
		req.Header.Set("Accept-Encoding", "gzip")
		w := newStrictHeaderRecorder()
		r.ServeHTTP(w, req)

		if w.Body.Len() != 0 {
			t.Fatalf("expected no body for HEAD, got %q", w.Body.String())
		}
		if got := w.Result().Header.Get("Content-Encoding"); got != "" {
			t.Fatalf("expected identity headers on HEAD overflow, got %s", got)
		}
	})
}

func pseudoRandomASCII(n int) string {
	buf := make([]byte, n)
	var x uint32 = 1
	for i := range buf {
		x = x*1664525 + 1013904223
		buf[i] = byte('a' + (x % 26))
	}
	return string(buf)
}

type strictHeaderRecorder struct {
	header      http.Header
	committed   http.Header
	Body        bytes.Buffer
	statusCode  int
	wroteHeader bool
}

func newStrictHeaderRecorder() *strictHeaderRecorder {
	return &strictHeaderRecorder{
		header:     make(http.Header),
		statusCode: http.StatusOK,
	}
}

func (w *strictHeaderRecorder) Header() http.Header {
	return w.header
}

func (w *strictHeaderRecorder) WriteHeader(statusCode int) {
	if w.wroteHeader {
		return
	}
	w.wroteHeader = true
	w.statusCode = statusCode
	w.committed = cloneHeader(w.header)
}

func (w *strictHeaderRecorder) Write(p []byte) (int, error) {
	if !w.wroteHeader {
		w.WriteHeader(http.StatusOK)
	}
	return w.Body.Write(p)
}

func (w *strictHeaderRecorder) Result() *http.Response {
	header := w.committed
	if header == nil {
		header = cloneHeader(w.header)
	}
	return &http.Response{
		StatusCode: w.statusCode,
		Header:     header,
		Body:       io.NopCloser(bytes.NewReader(w.Body.Bytes())),
	}
}

func cloneHeader(src http.Header) http.Header {
	dst := make(http.Header, len(src))
	for key, values := range src {
		dst[key] = append([]string(nil), values...)
	}
	return dst
}
