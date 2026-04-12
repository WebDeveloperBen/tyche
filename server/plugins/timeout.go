package plugins

import (
	"context"
	"net/http"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

type TimeoutConfig struct {
	Timeout time.Duration
}

func Timeout(cfg ...TimeoutConfig) server.Middleware {
	c := TimeoutConfig{Timeout: 30 * time.Second}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	m := &timeoutMiddleware{timeout: c.Timeout}
	return m.Middleware()
}

func TimeoutWithDefaults() server.Middleware {
	m := &timeoutMiddleware{timeout: 30 * time.Second}
	return m.Middleware()
}

type timeoutMiddleware struct {
	timeout time.Duration
}

func (m *timeoutMiddleware) Register(r *server.Router) {
	r.Use(m.Middleware())
}

type timeoutResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
}

func (tw *timeoutResponseWriter) WriteHeader(statusCode int) {
	tw.wroteHeader = true
	tw.ResponseWriter.WriteHeader(statusCode)
}

func (tw *timeoutResponseWriter) Written() bool {
	return tw.wroteHeader
}

func (m *timeoutMiddleware) Middleware() server.Middleware {
	return func(next server.HandlerFunc) server.HandlerFunc {
		return func(w http.ResponseWriter, r *http.Request) error {
			ctx, cancel := context.WithTimeout(r.Context(), m.timeout)
			defer cancel()

			tw := &timeoutResponseWriter{ResponseWriter: w}

			type result struct {
				err error
			}
			resCh := make(chan result, 1)

			go func() {
				resCh <- result{err: next(tw, r.WithContext(ctx))}
			}()

			select {
			case res := <-resCh:
				return res.err
			case <-ctx.Done():
				if tw.wroteHeader {
					return server.SilentHTTPError(http.StatusGatewayTimeout, "Request timeout")
				}
				return server.NewHTTPError(http.StatusGatewayTimeout, "Request timeout")
			}
		}
	}
}
