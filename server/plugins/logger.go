package plugins

import (
	"bytes"
	"io"
	"log"
	"log/slog"
	"net/http"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

const (
	defaultMaxBodySize = 64 * 1024
)

type statusRecorder struct {
	http.ResponseWriter
	status  int
	written bool
}

func (sr *statusRecorder) WriteHeader(status int) {
	if !sr.written {
		sr.status = status
		sr.ResponseWriter.WriteHeader(status)
	}
}

func (sr *statusRecorder) Write(b []byte) (int, error) {
	if !sr.written {
		sr.written = true
		sr.status = http.StatusOK
	}
	return sr.ResponseWriter.Write(b)
}

type LoggerConfig struct {
	LogFunc     func(method, path, query string, status int, duration time.Duration, err error)
	MaxBodySize int
	WithBody    bool
	WithQuery   bool
	DurationMs  bool
}

func Logger(cfg ...LoggerConfig) server.Middleware {
	c := LoggerConfig{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	m := &loggerMiddleware{config: c}
	return m.Middleware()
}

func LoggerWithDefaults(cfg ...LoggerConfig) server.Middleware {
	c := LoggerConfig{}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	if c.LogFunc == nil {
		c.WithQuery = true
		c.DurationMs = true
		c.LogFunc = func(method, path, query string, status int, duration time.Duration, err error) {
			slog.Info(
				"request",
				"method", method,
				"path", path,
				"query", query,
				"status", status,
				"duration_ms", duration.Milliseconds(),
				"error", err,
			)
		}
	}
	m := &loggerMiddleware{config: c}
	return m.Middleware()
}

type loggerMiddleware struct {
	config LoggerConfig
}

func (m *loggerMiddleware) Middleware() server.Middleware {
	maxBodySize := m.config.MaxBodySize
	if maxBodySize <= 0 {
		maxBodySize = defaultMaxBodySize
	}

	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			start := time.Now()

			var body []byte
			if m.config.WithBody && r.Body != nil {
				limitedReader := &io.LimitedReader{R: r.Body, N: int64(maxBodySize) + 1}
				body, _ = io.ReadAll(limitedReader)
				r.Body = io.NopCloser(bytes.NewBuffer(body))
				if limitedReader.N == 0 && body != nil {
					r.Header.Set("X-Body-Truncated", "true")
				}
			}

			sr := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
			err := next(sr, r)

			duration := time.Since(start)
			query := ""
			if m.config.WithQuery {
				query = r.URL.RawQuery
			}

			if m.config.LogFunc != nil {
				m.config.LogFunc(r.Method, r.URL.Path, query, sr.status, duration, err)
			} else {
				log.Printf("[%s] %s %s %s %d %v", duration.String(), r.Method, r.URL.Path, query, sr.status, err) //nolint:gosec
			}

			return err
		}
	}
}
