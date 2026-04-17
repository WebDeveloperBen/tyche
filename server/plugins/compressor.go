package plugins

import (
	"bufio"
	"bytes"
	"compress/gzip"
	"errors"
	"io"
	"net"
	"net/http"
	"os"
	"strconv"
	"strings"
	"sync"

	"github.com/andybalholm/brotli"
	"github.com/webdeveloperben/tyche/server"
)

const bufferedResponseMemoryLimit int64 = 1 << 20

var errCompressedSizeLimitExceeded = errors.New("compressed output exceeds configured size limit")
var errBufferedResponseLimitExceeded = errors.New("buffered response exceeds configured size limit")

type CompressorConfig struct {
	Level                   int
	ContentTypes            []string
	MaxCompressedSize       int64
	MaxBufferedResponseSize int64
}

type compressorMiddleware struct {
	level                   int
	contentTypes            map[string]struct{}
	contentWildcard         map[string]struct{}
	maxCompressedSize       int64
	maxBufferedResponseSize int64
	poolBrotli              *sync.Pool
	poolGzip                *sync.Pool
}

func Compressor(cfg ...CompressorConfig) server.Middleware {
	c := CompressorConfig{
		Level: 5,
		ContentTypes: []string{
			"text/html", "text/css", "text/plain", "text/javascript",
			"application/javascript", "application/x-javascript", "application/json",
			"application/atom+xml", "application/rss+xml", "image/svg+xml",
		},
		MaxCompressedSize:       10 << 20,
		MaxBufferedResponseSize: 8 << 20,
	}
	if len(cfg) > 0 {
		if cfg[0].Level != 0 {
			c.Level = cfg[0].Level
		}
		if len(cfg[0].ContentTypes) > 0 {
			c.ContentTypes = cfg[0].ContentTypes
		}
		if cfg[0].MaxCompressedSize >= 0 {
			c.MaxCompressedSize = cfg[0].MaxCompressedSize
		}
		if cfg[0].MaxBufferedResponseSize > 0 {
			c.MaxBufferedResponseSize = cfg[0].MaxBufferedResponseSize
		}
	}

	contentTypes := make(map[string]struct{}, len(c.ContentTypes))
	contentWildcard := make(map[string]struct{}, len(c.ContentTypes))
	for _, t := range c.ContentTypes {
		if before, ok := strings.CutSuffix(t, "/*"); ok {
			contentWildcard[before] = struct{}{}
			continue
		}
		contentTypes[t] = struct{}{}
	}

	m := &compressorMiddleware{
		level:                   c.Level,
		contentTypes:            contentTypes,
		contentWildcard:         contentWildcard,
		maxCompressedSize:       c.MaxCompressedSize,
		maxBufferedResponseSize: c.MaxBufferedResponseSize,
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
			encoding, identityAllowed := m.selectEncoding(r.Header.Get("Accept-Encoding"))
			useBuffer := m.maxCompressedSize > 0
			if encoding == "" && !identityAllowed {
				return server.NewHTTPError(http.StatusNotAcceptable, http.StatusText(http.StatusNotAcceptable))
			}

			cw := &compressResponseWriter{
				ResponseWriter:  w,
				contentTypes:    m.contentTypes,
				contentWildcard: m.contentWildcard,
				encoding:        encoding,
				identityAllowed: identityAllowed,
				useBuffer:       useBuffer,
				requestMethod:   r.Method,
				statusCode:      http.StatusOK,
			}

			if useBuffer {
				cw.body = newSpillBuffer(bufferedResponseMemoryLimit, m.maxBufferedResponseSize)
				defer cw.body.Close()
			}

			if !useBuffer && encoding != "" {
				cw.streamWriter, cw.streamCloser, cw.streamFlusher = m.getStreamWriter(encoding, w)
				defer func() {
					closeErr := cw.closeStreamWriter()
					if closeErr == nil {
						m.releaseStreamWriter(cw)
						return
					}
					if cw.streamErr == nil {
						cw.streamErr = closeErr
					}
				}()
			}

			err := next(cw, r)

			if useBuffer {
				if errors.Is(err, errBufferedResponseLimitExceeded) {
					return nil
				}
				if err != nil {
					return err
				}
				if cw.terminalErr != nil {
					return cw.terminalErr
				}
				if cw.passthrough {
					return nil
				}
				return m.finishBuffered(cw, r)
			}

			if err == nil {
				err = cw.streamErr
			}
			return err
		}
	}
}

func (m *compressorMiddleware) finishBuffered(cw *compressResponseWriter, r *http.Request) error {
	if cw.hijacked {
		return nil
	}

	status := cw.statusCode
	bodyAllowed := responseAllowsBody(status, r.Method)
	rawSize := cw.body.Size()
	if cw.Header().Get("Content-Type") == "" && rawSize > 0 {
		if detected, err := cw.body.DetectContentType(); err == nil && detected != "" {
			cw.Header().Set("Content-Type", detected)
		}
	}
	shouldCompress := bodyAllowed && rawSize > 0 && cw.shouldCompress(status)

	if shouldCompress {
		compressed, err := m.compressBufferedBody(cw.encoding, cw.body)
		switch {
		case err == nil:
			setCompressedHeaders(cw.Header(), cw.encoding, int64(len(compressed)))
			cw.ResponseWriter.WriteHeader(status)
			cw.wroteToClient = true
			if _, writeErr := cw.ResponseWriter.Write(compressed); writeErr != nil {
				return writeErr
			}
			return nil
		case errors.Is(err, errCompressedSizeLimitExceeded):
			if !cw.identityAllowed {
				return server.NewHTTPError(http.StatusNotAcceptable, http.StatusText(http.StatusNotAcceptable))
			}
			addVaryAcceptEncoding(cw.Header())
		default:
			return err
		}
	}

	if bodyAllowed {
		cw.Header().Set("Content-Length", strconv.FormatInt(rawSize, 10))
	} else {
		cw.Header().Del("Content-Length")
	}

	cw.ResponseWriter.WriteHeader(status)
	cw.wroteToClient = true
	if !bodyAllowed || rawSize == 0 {
		return nil
	}

	reader, err := cw.body.Reader()
	if err != nil {
		return err
	}
	_, err = io.Copy(cw.ResponseWriter, reader)
	return err
}

func (m *compressorMiddleware) compressBufferedBody(encoding string, body *spillBuffer) ([]byte, error) {
	reader, err := body.Reader()
	if err != nil {
		return nil, err
	}

	dst := &boundedBuffer{limit: m.maxCompressedSize}

	switch encoding {
	case "br":
		bw := m.poolBrotli.Get().(*brotliWriter)
		bw.Reset(dst)
		_, copyErr := io.Copy(bw, reader)
		closeErr := bw.Close()
		if copyErr == nil && closeErr == nil {
			bw.Reset(io.Discard)
			m.poolBrotli.Put(bw)
		}
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	case "gzip":
		gw := m.poolGzip.Get().(*gzipWriter)
		_ = gw.Reset(dst)
		_, copyErr := io.Copy(gw, reader)
		closeErr := gw.Close()
		if copyErr == nil && closeErr == nil {
			_ = gw.Reset(io.Discard)
			m.poolGzip.Put(gw)
		}
		if copyErr != nil {
			return nil, copyErr
		}
		if closeErr != nil {
			return nil, closeErr
		}
	default:
		return nil, nil
	}

	return dst.Bytes(), nil
}

func (m *compressorMiddleware) getStreamWriter(encoding string, dst io.Writer) (io.Writer, io.Closer, streamFlusher) {
	switch encoding {
	case "br":
		bw := m.poolBrotli.Get().(*brotliWriter)
		bw.Reset(dst)
		return bw, bw, bw
	case "gzip":
		gw := m.poolGzip.Get().(*gzipWriter)
		_ = gw.Reset(dst)
		return gw, gw, gw
	default:
		return nil, nil, nil
	}
}

func (m *compressorMiddleware) releaseStreamWriter(cw *compressResponseWriter) {
	switch cw.encoding {
	case "br":
		if bw, ok := cw.streamWriter.(*brotliWriter); ok {
			bw.Reset(io.Discard)
			m.poolBrotli.Put(bw)
		}
	case "gzip":
		if gw, ok := cw.streamWriter.(*gzipWriter); ok {
			_ = gw.Reset(io.Discard)
			m.poolGzip.Put(gw)
		}
	}
}

func (m *compressorMiddleware) selectEncoding(header string) (string, bool) {
	if header == "" {
		return "", true
	}

	type acceptEncodingPrefs struct {
		br       float64
		gzip     float64
		wildcard float64
		identity float64
		hasBr    bool
		hasGzip  bool
		hasStar  bool
		hasID    bool
	}

	prefs := acceptEncodingPrefs{}

	for _, part := range strings.Split(strings.ToLower(header), ",") {
		token, params, _ := strings.Cut(strings.TrimSpace(part), ";")
		token = strings.TrimSpace(token)
		if token != "br" && token != "gzip" && token != "*" && token != "identity" {
			continue
		}

		q := 1.0
		if params != "" {
			for _, param := range strings.Split(params, ";") {
				key, value, ok := strings.Cut(strings.TrimSpace(param), "=")
				if !ok || strings.TrimSpace(key) != "q" {
					continue
				}
				parsed, err := strconv.ParseFloat(strings.TrimSpace(value), 64)
				if err == nil {
					q = parsed
				}
			}
		}
		switch token {
		case "br":
			prefs.br = q
			prefs.hasBr = true
		case "gzip":
			prefs.gzip = q
			prefs.hasGzip = true
		case "*":
			prefs.wildcard = q
			prefs.hasStar = true
		case "identity":
			prefs.identity = q
			prefs.hasID = true
		}
	}

	candidateQ := func(explicit float64, hasExplicit bool) float64 {
		if hasExplicit {
			return explicit
		}
		if prefs.hasStar {
			return prefs.wildcard
		}
		return 0
	}

	bestEncoding := ""
	bestQ := 0.0

	brQ := candidateQ(prefs.br, prefs.hasBr)
	if brQ > bestQ {
		bestEncoding = "br"
		bestQ = brQ
	}
	gzipQ := candidateQ(prefs.gzip, prefs.hasGzip)
	if gzipQ > bestQ || (gzipQ == bestQ && gzipQ > 0 && bestEncoding == "") {
		bestEncoding = "gzip"
		bestQ = gzipQ
	}

	identityQ := 1.0
	if prefs.hasID {
		identityQ = prefs.identity
	} else if prefs.hasStar {
		identityQ = prefs.wildcard
	}

	return bestEncoding, identityQ > 0
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

func (b *brotliWriter) Flush() error {
	return b.w.Flush()
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

func (g *gzipWriter) Flush() error {
	return g.w.Flush()
}

func (g *gzipWriter) Close() error {
	return g.w.Close()
}

type streamFlusher interface {
	Flush() error
}

type compressResponseWriter struct {
	http.ResponseWriter
	body            *spillBuffer
	contentTypes    map[string]struct{}
	contentWildcard map[string]struct{}
	encoding        string
	identityAllowed bool
	streamWriter    io.Writer
	streamCloser    io.Closer
	streamFlusher   streamFlusher
	useBuffer       bool
	compressible    bool
	passthrough     bool
	terminalErr     error
	streamErr       error
	wroteHeader     bool
	wroteToClient   bool
	hijacked        bool
	requestMethod   string
	statusCode      int
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
		_, ok = cw.contentWildcard[base]
		return ok
	}
	return false
}

func (cw *compressResponseWriter) Written() bool {
	return cw.wroteToClient
}

func (cw *compressResponseWriter) WriteHeader(code int) {
	if cw.hijacked || cw.wroteHeader {
		return
	}

	cw.wroteHeader = true
	cw.statusCode = code
	cw.compressible = cw.shouldCompress(code)

	if cw.useBuffer {
		return
	}

	if cw.compressible {
		cw.prepareStreamingHeaders()
	}

	cw.wroteToClient = true
	cw.ResponseWriter.WriteHeader(code)
}

func (cw *compressResponseWriter) Write(p []byte) (int, error) {
	if cw.hijacked {
		return 0, http.ErrHijacked
	}
	if !cw.wroteHeader {
		if cw.Header().Get("Content-Type") == "" && len(p) > 0 {
			cw.Header().Set("Content-Type", http.DetectContentType(p))
		}
		cw.WriteHeader(http.StatusOK)
	}

	if cw.useBuffer {
		if cw.passthrough {
			if responseAllowsBody(cw.statusCode, cw.requestMethod) {
				cw.wroteToClient = true
				return cw.ResponseWriter.Write(p)
			}
			return len(p), nil
		}
		n, err := cw.body.Write(p)
		if errors.Is(err, errBufferedResponseLimitExceeded) {
			if !cw.identityAllowed {
				cw.terminalErr = server.NewHTTPError(http.StatusNotAcceptable, http.StatusText(http.StatusNotAcceptable))
				return n, cw.terminalErr
			}
			if flushErr := cw.startIdentityPassthrough(); flushErr != nil {
				return 0, flushErr
			}
			cw.passthrough = true
			if !responseAllowsBody(cw.statusCode, cw.requestMethod) || n == len(p) {
				return n, nil
			}
			cw.wroteToClient = true
			written, writeErr := cw.ResponseWriter.Write(p[n:])
			return n + written, writeErr
		}
		return n, err
	}

	if cw.compressible && cw.streamWriter != nil {
		n, err := cw.streamWriter.Write(p)
		if err != nil && cw.streamErr == nil {
			cw.streamErr = err
		}
		return n, err
	}

	cw.wroteToClient = true
	return cw.ResponseWriter.Write(p)
}

func (cw *compressResponseWriter) Flush() {
	if cw.hijacked {
		return
	}
	if cw.useBuffer {
		if cw.passthrough {
			if f, ok := cw.ResponseWriter.(http.Flusher); ok {
				f.Flush()
			}
		}
		return
	}
	if !cw.wroteHeader {
		cw.WriteHeader(http.StatusOK)
	}
	if cw.compressible && cw.streamFlusher != nil {
		if err := cw.streamFlusher.Flush(); err != nil && cw.streamErr == nil {
			cw.streamErr = err
			return
		}
	}
	if f, ok := cw.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

func (cw *compressResponseWriter) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	hj, ok := cw.ResponseWriter.(http.Hijacker)
	if !ok {
		return nil, nil, http.ErrNotSupported
	}
	if cw.useBuffer && cw.body != nil && cw.body.Size() > 0 {
		return nil, nil, errors.New("cannot hijack connection after buffered response write")
	}
	if !cw.useBuffer {
		if err := cw.closeStreamWriter(); err != nil {
			return nil, nil, err
		}
	}
	cw.hijacked = true
	cw.wroteToClient = true
	return hj.Hijack()
}

func (cw *compressResponseWriter) Push(target string, opts *http.PushOptions) error {
	if ps, ok := cw.ResponseWriter.(http.Pusher); ok {
		return ps.Push(target, opts)
	}
	return http.ErrNotSupported
}

func (cw *compressResponseWriter) Close() error {
	if c, ok := cw.ResponseWriter.(io.Closer); ok {
		return c.Close()
	}
	return nil
}

func (cw *compressResponseWriter) Unwrap() http.ResponseWriter {
	return cw.ResponseWriter
}

func (cw *compressResponseWriter) shouldCompress(status int) bool {
	if cw.encoding == "" {
		return false
	}
	if cw.Header().Get("Content-Encoding") != "" {
		return false
	}
	if !statusAllowsCompression(status) {
		return false
	}
	return cw.isCompressible()
}

func (cw *compressResponseWriter) prepareStreamingHeaders() {
	setCompressedHeaders(cw.Header(), cw.encoding, -1)
}

func (cw *compressResponseWriter) startIdentityPassthrough() error {
	if cw.passthrough {
		return nil
	}

	cw.Header().Del("Content-Encoding")
	if cw.compressible {
		addVaryAcceptEncoding(cw.Header())
	}
	cw.Header().Del("Content-Length")

	cw.ResponseWriter.WriteHeader(cw.statusCode)
	cw.wroteToClient = true

	if cw.body == nil || cw.body.Size() == 0 || !responseAllowsBody(cw.statusCode, cw.requestMethod) {
		cw.passthrough = true
		return nil
	}

	reader, err := cw.body.Reader()
	if err != nil {
		return err
	}
	if _, err := io.Copy(cw.ResponseWriter, reader); err != nil {
		return err
	}
	cw.passthrough = true
	return nil
}

func (cw *compressResponseWriter) closeStreamWriter() error {
	if cw.streamCloser == nil {
		return nil
	}
	closer := cw.streamCloser
	cw.streamCloser = nil
	return closer.Close()
}

func setCompressedHeaders(header http.Header, encoding string, contentLength int64) {
	header.Set("Content-Encoding", encoding)
	addVaryAcceptEncoding(header)
	if contentLength >= 0 {
		header.Set("Content-Length", strconv.FormatInt(contentLength, 10))
		return
	}
	header.Del("Content-Length")
}

func addVaryAcceptEncoding(header http.Header) {
	for _, value := range header.Values("Vary") {
		for _, part := range strings.Split(value, ",") {
			if strings.EqualFold(strings.TrimSpace(part), "Accept-Encoding") {
				return
			}
		}
	}
	header.Add("Vary", "Accept-Encoding")
}

func removeVaryAcceptEncoding(header http.Header) {
	values := header.Values("Vary")
	if len(values) == 0 {
		return
	}

	rebuilt := make([]string, 0, len(values))
	for _, value := range values {
		parts := strings.Split(value, ",")
		filtered := make([]string, 0, len(parts))
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if part == "" || strings.EqualFold(part, "Accept-Encoding") {
				continue
			}
			filtered = append(filtered, part)
		}
		if len(filtered) > 0 {
			rebuilt = append(rebuilt, strings.Join(filtered, ", "))
		}
	}

	header.Del("Vary")
	for _, value := range rebuilt {
		header.Add("Vary", value)
	}
}

func statusAllowsCompression(status int) bool {
	return status >= 200 && status != http.StatusNoContent && status != http.StatusNotModified
}

func responseAllowsBody(status int, method string) bool {
	if strings.EqualFold(method, http.MethodHead) {
		return false
	}
	if status >= 100 && status < 200 {
		return false
	}
	return status != http.StatusNoContent && status != http.StatusNotModified
}

type boundedBuffer struct {
	buf   bytes.Buffer
	limit int64
}

func (b *boundedBuffer) Write(p []byte) (int, error) {
	nextSize := int64(b.buf.Len()) + int64(len(p))
	if nextSize > b.limit {
		allowed := int(b.limit - int64(b.buf.Len()))
		if allowed > 0 {
			_, _ = b.buf.Write(p[:allowed])
			return allowed, errCompressedSizeLimitExceeded
		}
		return 0, errCompressedSizeLimitExceeded
	}
	return b.buf.Write(p)
}

func (b *boundedBuffer) Bytes() []byte {
	return b.buf.Bytes()
}

type spillBuffer struct {
	memLimit int64
	maxSize  int64
	mem      bytes.Buffer
	file     *os.File
	size     int64
}

func newSpillBuffer(memLimit int64, maxSize int64) *spillBuffer {
	return &spillBuffer{memLimit: memLimit, maxSize: maxSize}
}

func (b *spillBuffer) Write(p []byte) (int, error) {
	if b.maxSize > 0 && b.size+int64(len(p)) > b.maxSize {
		allowed := int(b.maxSize - b.size)
		if allowed > 0 {
			n, err := b.writeWithinLimit(p[:allowed])
			if err != nil {
				return n, err
			}
			return n, errBufferedResponseLimitExceeded
		}
		return 0, errBufferedResponseLimitExceeded
	}
	return b.writeWithinLimit(p)
}

func (b *spillBuffer) writeWithinLimit(p []byte) (int, error) {
	if b.file == nil && b.size+int64(len(p)) <= b.memLimit {
		n, err := b.mem.Write(p)
		b.size += int64(n)
		return n, err
	}

	if b.file == nil {
		file, err := os.CreateTemp("", "tyche-compressor-*")
		if err != nil {
			return 0, err
		}
		if _, err := file.Write(b.mem.Bytes()); err != nil {
			name := file.Name()
			_ = file.Close()
			_ = os.Remove(name)
			return 0, err
		}
		b.mem.Reset()
		b.file = file
	}

	n, err := b.file.Write(p)
	b.size += int64(n)
	return n, err
}

func (b *spillBuffer) Size() int64 {
	return b.size
}

func (b *spillBuffer) Reader() (io.Reader, error) {
	if b.file == nil {
		return bytes.NewReader(b.mem.Bytes()), nil
	}
	if _, err := b.file.Seek(0, io.SeekStart); err != nil {
		return nil, err
	}
	return b.file, nil
}

func (b *spillBuffer) DetectContentType() (string, error) {
	if b.size == 0 {
		return "", nil
	}
	sniffLen := int(b.size)
	if sniffLen > 512 {
		sniffLen = 512
	}
	if b.file == nil {
		return http.DetectContentType(b.mem.Bytes()[:sniffLen]), nil
	}

	buf := make([]byte, sniffLen)
	if _, err := b.file.Seek(0, io.SeekStart); err != nil {
		return "", err
	}
	n, err := io.ReadFull(b.file, buf)
	if err != nil && !errors.Is(err, io.ErrUnexpectedEOF) {
		return "", err
	}
	_, seekErr := b.file.Seek(0, io.SeekEnd)
	if seekErr != nil {
		return "", seekErr
	}
	return http.DetectContentType(buf[:n]), nil
}

func (b *spillBuffer) Close() error {
	if b.file == nil {
		return nil
	}
	name := b.file.Name()
	err := b.file.Close()
	removeErr := os.Remove(name)
	if err != nil {
		return err
	}
	return removeErr
}
