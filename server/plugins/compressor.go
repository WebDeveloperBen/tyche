package plugins

import (
	"bufio"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/webdeveloperben/tyche/server"
)

var ErrCompressedSizeLimitExceeded = errors.New("compressed output exceeds configured size limit")

type CompressorConfig struct {
	Level             int
	ContentTypes      []string
	MaxCompressedSize int64
}

type compressorMiddleware struct {
	level             int
	contentTypes      map[string]struct{}
	contentWildcard   map[string]struct{}
	maxCompressedSize int64
	poolBrotli        *sync.Pool
	poolGzip          *sync.Pool
}

func Compressor(cfg ...CompressorConfig) server.Middleware {
	c := CompressorConfig{
		Level: 5,
		ContentTypes: []string{
			"text/html", "text/css", "text/plain", "text/javascript",
			"application/javascript", "application/x-javascript", "application/json",
			"application/atom+xml", "application/rss+xml", "image/svg+xml",
		},
		MaxCompressedSize: 10 << 20,
	}
	if len(cfg) > 0 {
		if cfg[0].Level != 0 {
			c.Level = cfg[0].Level
		}
		if len(cfg[0].ContentTypes) > 0 {
			c.ContentTypes = cfg[0].ContentTypes
		}
		if cfg[0].MaxCompressedSize > 0 {
			c.MaxCompressedSize = cfg[0].MaxCompressedSize
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
		level:             c.Level,
		contentTypes:      contentTypes,
		contentWildcard:   contentWildcard,
		maxCompressedSize: c.MaxCompressedSize,
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
				ResponseWriter:    w,
				w:                 w,
				contentTypes:      m.contentTypes,
				contentWildcard:   m.contentWildcard,
				encoding:          encoding,
				compressible:      false,
				maxCompressedSize: m.maxCompressedSize,
			}

			if encoding != "" {
				switch encoding {
				case "br":
					bw := m.poolBrotli.Get().(*brotliWriter)
					bw.Reset(w)
					bw.limit = m.maxCompressedSize
					cw.w = bw
					cw.compressWriter = bw
					defer m.poolBrotli.Put(bw)
				case "gzip":
					gw := m.poolGzip.Get().(*gzipWriter)
					gw.Reset(w)
					gw.limit = m.maxCompressedSize
					cw.w = gw
					cw.compressWriter = gw
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

type limitWriter struct {
	w     io.Writer
	limit int64
	n     int64
}

func (lw *limitWriter) Write(p []byte) (int, error) {
	if lw.n+int64(len(p)) > lw.limit {
		n, _ := lw.w.Write(p[:lw.limit-lw.n])
		lw.n = lw.limit
		return n, ErrCompressedSizeLimitExceeded
	}
	n, err := lw.w.Write(p)
	lw.n += int64(n)
	return n, err
}

type brotliWriter struct {
	w     *brotli.Writer
	limit int64
	n     int64
}

func (b *brotliWriter) Write(p []byte) (int, error) {
	if b.limit > 0 && b.n+int64(len(p)) > b.limit {
		before := b.limit - b.n
		if before <= 0 {
			return 0, ErrCompressedSizeLimitExceeded
		}
		n, err := b.w.Write(p[:before])
		b.n += int64(n)
		if b.n >= b.limit {
			return n, ErrCompressedSizeLimitExceeded
		}
		return n, err
	}
	n, err := b.w.Write(p)
	b.n += int64(n)
	return n, err
}

func (b *brotliWriter) Reset(w io.Writer) {
	b.w.Reset(w)
	b.n = 0
}

func (b *brotliWriter) Close() error {
	return b.w.Close()
}

type gzipWriter struct {
	w     *gzip.Writer
	limit int64
	n     int64
}

func (g *gzipWriter) Write(p []byte) (int, error) {
	if g.limit > 0 && g.n+int64(len(p)) > g.limit {
		before := g.limit - g.n
		if before <= 0 {
			return 0, ErrCompressedSizeLimitExceeded
		}
		n, err := g.w.Write(p[:before])
		g.n += int64(n)
		if g.n >= g.limit {
			return n, ErrCompressedSizeLimitExceeded
		}
		return n, err
	}
	n, err := g.w.Write(p)
	g.n += int64(n)
	return n, err
}

func (g *gzipWriter) Reset(w io.Writer) error {
	g.w.Reset(w)
	g.n = 0
	return nil
}

func (g *gzipWriter) Close() error {
	return g.w.Close()
}

type compressResponseWriter struct {
	http.ResponseWriter
	w                 io.Writer
	compressWriter    io.Writer
	contentTypes      map[string]struct{}
	contentWildcard   map[string]struct{}
	encoding          string
	wroteHeader       bool
	compressible      bool
	maxCompressedSize int64
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
