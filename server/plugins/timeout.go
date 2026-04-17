package plugins

import (
	"context"
	"net/http"
	"sync"
	"time"

	"github.com/webdeveloperben/tyche/server"
)

type TimeoutConfig struct {
	Timeout        time.Duration
	HandlerTimeout time.Duration
}

func Timeout(cfg ...TimeoutConfig) server.Middleware {
	c := TimeoutConfig{Timeout: 30 * time.Second, HandlerTimeout: 100 * time.Millisecond}
	if len(cfg) > 0 {
		c = cfg[0]
	}
	m := &timeoutMiddleware{timeout: c.Timeout, handlerTimeout: c.HandlerTimeout}
	return m.Middleware()
}

func TimeoutWithDefaults() server.Middleware {
	m := &timeoutMiddleware{timeout: 30 * time.Second, handlerTimeout: 100 * time.Millisecond}
	return m.Middleware()
}

type timeoutMiddleware struct {
	timeout        time.Duration
	handlerTimeout time.Duration
}

func (m *timeoutMiddleware) Register(r *server.Router) {
	r.Use(m.Middleware())
}

type timeoutResponseWriter struct {
	http.ResponseWriter
	wroteHeader bool
	wroteBody   bool
}

func (tw *timeoutResponseWriter) WriteHeader(statusCode int) {
	tw.wroteHeader = true
	tw.ResponseWriter.WriteHeader(statusCode)
}

func (tw *timeoutResponseWriter) Write(p []byte) (int, error) {
	tw.wroteHeader = true
	tw.wroteBody = true
	return tw.ResponseWriter.Write(p)
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

			var handlerDone sync.WaitGroup
			handlerDone.Add(1)

			type result struct {
				err error
			}
			resCh := make(chan result, 1)

			go func() {
				defer handlerDone.Done()
				resCh <- result{err: next(tw, r.WithContext(ctx))}
			}()

			select {
			case res := <-resCh:
				return res.err
			case <-ctx.Done():
				handlerDone.Wait()
				if tw.wroteHeader {
					return nil
				}
				return server.NewHTTPError(http.StatusGatewayTimeout, "Request timeout")
			case <-r.Context().Done():
				handlerDone.Wait()
				return r.Context().Err()
			}
		}
	}
}
