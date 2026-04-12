package plugins

import (
	"bufio"
	"compress/gzip"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/webdeveloperben/tyche/server"
)

type CompressorConfig struct {
	Level        int
	ContentTypes []string
}

type compressorMiddleware struct {
	level           int
	contentTypes    map[string]struct{}
	contentWildcard map[string]struct{}
	poolBrotli      *sync.Pool
	poolGzip        *sync.Pool
}

func Compressor(cfg ...CompressorConfig) server.Middleware {
	c := CompressorConfig{
		Level: 5,
		ContentTypes: []string{
			"text/html", "text/css", "text/plain", "text/javascript",
			"application/javascript", "application/x-javascript", "application/json",
			"application/atom+xml", "application/rss+xml", "image/svg+xml",
		},
	}
	if len(cfg) > 0 {
		if cfg[0].Level != 0 {
			c.Level = cfg[0].Level
		}
		if len(cfg[0].ContentTypes) > 0 {
			c.ContentTypes = cfg[0].ContentTypes
		}
	}

	contentTypes := make(map[string]struct{})
	contentWildcard := make(map[string]struct{})
	for _, t := range c.ContentTypes {
		if before, ok := strings.CutSuffix(t, "/*"); ok {
			contentWildcard[before] = struct{}{}
		} else {
			contentTypes[t] = struct{}{}
		}
	}

	m := &compressorMiddleware{
		level:           c.Level,
		contentTypes:    contentTypes,
		contentWildcard: contentWildcard,
	}

	m.poolBrotli = &sync.Pool{
		New: func() any {
			return &brotliWriter{w: brotli.NewWriter(nil)}
		},
	}
	m.poolGzip = &sync.Pool{
		New: func() any {
			gz, _ := gzip.NewWriterLevel(nil, m.level)
			return &gzipWriter{w: gz}
		},
	}

	return m.Middleware()
}

func CompressorWithDefaults() server.Middleware {
	return Compressor(CompressorConfig{})
}

func (m *compressorMiddleware) Register(r *server.Router) {
	r.Use(m.Middleware())
}

func (m *compressorMiddleware) Middleware() server.Middleware {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			encoding := m.selectEncoding(r.Header.Get("Accept-Encoding"))

			cw := &compressResponseWriter{
				ResponseWriter:  w,
				w:               w,
				contentTypes:    m.contentTypes,
				contentWildcard: m.contentWildcard,
				encoding:        encoding,
				compressible:    false,
			}

			if encoding != "" {
				switch encoding {
				case "br":
					bw := m.poolBrotli.Get().(*brotliWriter)
					bw.Reset(w)
					cw.w = bw
					defer m.poolBrotli.Put(bw)
				case "gzip":
					gw := m.poolGzip.Get().(*gzipWriter)
					gw.Reset(w)
					cw.w = gw
					defer m.poolGzip.Put(gw)
				}
			}

			err := next(cw, r)
			if cw.compressible {
				if c, ok := cw.w.(io.Closer); ok {
					c.Close()
				}
			}
			return err
		}
	}
}

func (m *compressorMiddleware) selectEncoding(header string) string {
	if header == "" {
		return ""
	}
	accepted := strings.Split(strings.ToLower(header), ",")
	for _, enc := range []string{"br", "gzip"} {
		for _, a := range accepted {
			if strings.Contains(a, enc) {
				return enc
			}
		}
	}
	return ""
}

type brotliWriter struct {
	w *brotli.Writer
}

func (b *brotliWriter) Write(p []byte) (int, error) {
	return b.w.Write(p)
}

func (b *brotliWriter) Reset(w io.Writer) {
	b.w.Reset(w)
}

func (b *brotliWriter) Close() error {
	return b.w.Close()
}

type gzipWriter struct {
	w *gzip.Writer
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	return g.w.Write(p)
}

func (g *gzipWriter) Reset(w io.Writer) error {
	g.w.Reset(w)
	return nil
}

func (g *gzipWriter) Close() error {
	return g.w.Close()
}

type compressResponseWriter struct {
	http.ResponseWriter
	w               io.Writer
	contentTypes    map[string]struct{}
	contentWildcard map[string]struct{}
	encoding        string
	wroteHeader     bool
	compressible    bool
}

func (cw *compressResponseWriter) isCompressible() bool {
	contentType := cw.Header().Get("Content-Type")
	if contentType == "" {
		return false
	}
	contentType, _, _ = strings.Cut(contentType, ";")

	if _, ok := cw.contentTypes[contentType]; ok {
		return true
	}
	if base, _, ok := strings.Cut(contentType, "/"); ok {
		if _, ok := cw.contentWildcard[base]; ok {
			return true
		}
	}
	return false
}

func (cw *compressResponseWriter) WriteHeader(code int) {
	if cw.wroteHeader {
		cw.ResponseWriter.WriteHeader(code)
		return
	}
	cw.wroteHeader = true
	defer cw.ResponseWriter.WriteHeader(code)

	if cw.Header().Get("Content-Encoding") != "" {
		return
	}

	if !cw.isCompressible() {
		return
	}

	if cw.encoding != "" {
		cw.compressible = true
		cw.Header().Set("Content-Encoding", cw.encoding)
		cw.Header().Add("Vary", "Accept-Encoding")
		cw.Header().Del("Content-Length")
	}
}

func (cw *compressResponseWriter) Write(p []byte) (int, error) {
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}
	return cw.writer().Write(p)
}

func (cw *compressResponseWriter) writer() io.Writer {
	if cw.compressible {
		return cw.w
	}
	return cw.ResponseWriter
}

func (cw *compressResponseWriter) Flush() {
	if f, ok := cw.writer().(http.Flusher); ok {
		f.Flush()
	}
}

func (cw *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if hj, ok := cw.writer().(http.Hijacker); ok {
		return hj.Hijack()
	}
	return nil, nil, http.ErrNotSupported
}

func (cw *compressResponseWriter) Push(target string, opts *http.PushOptions) error {
	if ps, ok := cw.writer().(http.Pusher); ok {
		return ps.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (cw *compressResponseWriter) Close() error {
	if c, ok := cw.writer().(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (cw *compressResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}
